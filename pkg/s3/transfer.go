package s3

import (
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/cleanup"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"golang.org/x/sync/errgroup"
)

// syncParallelism caps the number of concurrent per-object transfers.
const syncParallelism = 16

// remoteObject is a single S3 object selected for transfer. versionID is set only on the point-in-time
// download path, where a specific (non-latest) version may be the one current as of the consistency point.
type remoteObject struct {
	key          string // full S3 key
	relPath      string // key relative to the prefix, slash-separated; the path under the local directory
	versionID    *string
	size         int64
	lastModified time.Time
}

// Sync makes the destination an exact mirror of the source: changed/new objects are transferred and items
// missing from the source are removed. See Runtime.Sync for the asOf semantics and selectObjectsAsOf for
// the point-in-time reconstruction.
func (lr *LocalRuntime) Sync(ctx *contexts.Context, credentials CredentialsInterface, src, dest string, asOf time.Time) (err error) {
	ctx.Log.With("src", src, "dest", dest).Info("Syncing files")
	defer ctx.Log.Info("Finished syncing files", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	client := lr.newS3Client(credentials.AWSConfig(), func(o *s3.Options) {
		// Endpoint and path-style are S3 client options in the v2 SDK rather than fields on aws.Config.
		if endpoint := credentials.GetEndpoint(); endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
		o.UsePathStyle = credentials.GetS3ForcePathStyle()
	})

	srcPath, srcIsS3, err := parseS3Path(src)
	if err != nil {
		return trace.Wrap(err, "failed to parse source path")
	}

	destPath, destIsS3, err := parseS3Path(dest)
	if err != nil {
		return trace.Wrap(err, "failed to parse destination path")
	}

	switch {
	case srcIsS3 && !destIsS3:
		return trace.Wrap(lr.download(ctx.Child(), client, srcPath, dest, asOf), "failed to download from %q to %q", src, dest)
	case !srcIsS3 && destIsS3:
		return trace.Wrap(lr.upload(ctx.Child(), client, src, destPath), "failed to upload from %q to %q", src, dest)
	case srcIsS3 && destIsS3:
		return trace.Errorf("s3-to-s3 sync is not supported")
	default:
		return trace.Errorf("local-to-local sync is not supported")
	}
}

// download syncs an S3 prefix down to a local directory. When asOf is non-zero and the bucket has
// versioning enabled, the directory is reconstructed as of asOf; otherwise it mirrors the latest state.
func (lr *LocalRuntime) download(ctx *contexts.Context, client s3API, src s3Path, destDir string, asOf time.Time) error {
	pointInTime := false
	if !asOf.IsZero() {
		enabled, err := bucketVersioningEnabled(ctx, client, src.bucket)
		switch {
		case err != nil:
			// Can't confirm versioning (unsupported API, or missing s3:GetBucketVersioning). Degrade to a
			// latest-state sync rather than fail the backup, but warn so the lost consistency isn't silent.
			ctx.Log.Warn("Failed to determine bucket versioning status; capturing latest object state instead of the "+
				"consistency point. This event is NOT guaranteed to be cross-resource consistent.",
				"bucket", src.bucket, "error", err)
		case enabled:
			pointInTime = true
		default:
			// Without versioning the bucket can't be rewound to the consistency point.
			ctx.Log.Warn("Bucket versioning is not enabled; capturing latest object state instead of the consistency point. "+
				"This event is NOT guaranteed to be cross-resource consistent — enable bucket versioning to resolve this.",
				"bucket", src.bucket)
		}
	}

	var (
		objects []remoteObject
		err     error
	)
	if pointInTime {
		ctx.Log.With("bucket", src.bucket, "asOf", asOf).Info("Capturing bucket as of the consistency point")
		objects, err = listObjectsAsOf(ctx, client, src, asOf)
	} else {
		objects, err = listLatestObjects(ctx, client, src)
	}
	if err != nil {
		return trace.Wrap(err, "failed to list objects in bucket %q", src.bucket)
	}

	keep := make(map[string]struct{}, len(objects))

	var g errgroup.Group
	g.SetLimit(syncParallelism)
	for _, obj := range objects {
		obj := obj
		keep[filepath.FromSlash(obj.relPath)] = struct{}{}
		g.Go(func() error {
			return downloadObject(ctx, client, src.bucket, obj, destDir)
		})
	}
	if err := g.Wait(); err != nil {
		return trace.Wrap(err, "failed to download one or more objects")
	}

	// Prune files that should no longer be present: deleted from the bucket since the last backup, or
	// (point-in-time) absent as of the consistency point. This keeps the destination an exact mirror,
	// which matters when the DR volume is an incremental clone of a previous backup.
	if err := removeExtraneousLocalFiles(destDir, keep); err != nil {
		return trace.Wrap(err, "failed to prune stale files from %q", destDir)
	}
	return nil
}

// downloadObject downloads a single object into destDir at its relative path. Existing identical files are
// skipped, and the object's modification time is preserved so re-runs are idempotent.
func downloadObject(ctx *contexts.Context, client s3API, bucket string, obj remoteObject, destDir string) error {
	target := filepath.Join(destDir, filepath.FromSlash(obj.relPath))

	info, err := os.Stat(target)
	if err == nil && info.Size() == obj.size && !obj.lastModified.After(info.ModTime()) {
		return nil // an up-to-date copy already exists
	} else if !os.IsNotExist(err) {
		return trace.Wrap(err, "failed to stat %q", target)
	}

	input := &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(obj.key)}
	if obj.versionID != nil {
		input.VersionId = obj.versionID
	}

	out, err := client.GetObject(ctx, input)
	if err != nil {
		return trace.Wrap(err, "failed to get object %q", obj.key)
	}
	defer cleanup.To(func(_ *contexts.Context) error { return out.Body.Close() }).
		WithErrMessage("failed to close response body for object %q", obj.key).
		Run()

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return trace.Wrap(err, "failed to create parent directory for %q", target)
	}

	f, err := os.Create(target)
	if err != nil {
		return trace.Wrap(err, "failed to create %q", target)
	}

	_, copyErr := io.Copy(f, out.Body)
	closeErr := f.Close()
	copyCloseErr := trace.NewAggregate(
		trace.Wrap(copyErr, "failed to write object %q to %q", obj.key, target),
		trace.Wrap(closeErr, "failed to close %q", target),
	)
	if copyCloseErr != nil {
		return copyCloseErr
	}

	if err := os.Chtimes(target, obj.lastModified, obj.lastModified); err != nil {
		return trace.Wrap(err, "failed to set modification time on %q", target)
	}

	return nil
}

// upload syncs a local directory up to an S3 prefix (latest-state), pruning objects with no local
// counterpart so the bucket mirrors the directory.
func (lr *LocalRuntime) upload(ctx *contexts.Context, client s3API, srcDir string, dest s3Path) error {
	localFiles, err := listLocalFiles(srcDir)
	if err != nil {
		return trace.Wrap(err, "failed to enumerate local files under %q", srcDir)
	}

	remoteObjects, err := listLatestObjects(ctx, client, dest)
	if err != nil {
		return trace.Wrap(err, "failed to list existing objects in bucket %q", dest.bucket)
	}

	remoteByRel := make(map[string]remoteObject, len(remoteObjects))
	for _, obj := range remoteObjects {
		remoteByRel[obj.relPath] = obj
	}

	localRel := make(map[string]struct{}, len(localFiles))

	var g errgroup.Group
	g.SetLimit(syncParallelism)
	for _, lf := range localFiles {
		localRel[lf.relPath] = struct{}{}

		if existing, ok := remoteByRel[lf.relPath]; ok && existing.size == lf.size && !lf.modTime.After(existing.lastModified) {
			continue // already up to date
		}
		g.Go(func() error {
			err := uploadObject(ctx, client, dest, lf)
			return trace.Wrap(err, "failed to upload %q to %q", lf.absPath, path.Join(dest.bucket, dest.prefix, lf.relPath))
		})
	}

	for _, obj := range remoteObjects {
		if _, ok := localRel[obj.relPath]; ok {
			continue
		}

		g.Go(func() error {
			_, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(dest.bucket),
				Key:    aws.String(obj.key),
			})
			return trace.Wrap(err, "failed to delete object %q", obj.key)
		})
	}

	return trace.Wrap(g.Wait(), "failed to upload or prune one or more objects")
}

// uploadObject uploads a single local file to its key under the destination prefix.
func uploadObject(ctx *contexts.Context, client s3API, dest s3Path, lf localFile) (err error) {
	key := path.Join(dest.prefix, filepath.ToSlash(lf.relPath))

	f, err := os.Open(lf.absPath)
	if err != nil {
		return trace.Wrap(err, "failed to open %q", lf.absPath)
	}

	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = trace.Wrap(closeErr, "failed to close %q", lf.absPath)
		}
	}()

	contentType, err := detectContentType(f, lf.absPath)
	if err != nil {
		return trace.Wrap(err, "failed to detect content type of %q", lf.absPath)
	}

	// Body is an *os.File (an io.ReadSeeker), so the SDK can compute the payload signature and rewind on
	// retry without buffering the file in memory.
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(dest.bucket),
		Key:         aws.String(key),
		Body:        f,
		ContentType: aws.String(contentType),
	})
	return trace.Wrap(err, "failed to upload %q to %q", lf.absPath, key)
}

// bucketVersioningEnabled reports whether the bucket has versioning enabled (as opposed to never enabled
// or suspended). Only an enabled bucket can be reconstructed to a past instant.
func bucketVersioningEnabled(ctx *contexts.Context, client s3API, bucket string) (bool, error) {
	out, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: aws.String(bucket)})
	if err != nil {
		return false, trace.Wrap(err, "failed to get versioning configuration for bucket %q", bucket)
	}

	return out.Status == types.BucketVersioningStatusEnabled, nil
}

// listLatestObjects lists the current objects under the prefix.
func listLatestObjects(ctx *contexts.Context, client s3API, src s3Path) ([]remoteObject, error) {
	var objects []remoteObject
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(src.bucket),
		Prefix: aws.String(src.prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, trace.Wrap(err, "failed to list objects")
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			rel := relativeKey(src.prefix, key)
			if rel == "" {
				continue
			}

			objects = append(objects, remoteObject{
				key:          key,
				relPath:      rel,
				size:         aws.ToInt64(obj.Size),
				lastModified: aws.ToTime(obj.LastModified),
			})
		}
	}

	return objects, nil
}

// listObjectsAsOf lists every object version and delete marker under the prefix, then resolves the set of
// objects current as of asOf via selectObjectsAsOf.
func listObjectsAsOf(ctx *contexts.Context, client s3API, src s3Path, asOf time.Time) ([]remoteObject, error) {
	var (
		versions      []types.ObjectVersion
		deleteMarkers []types.DeleteMarkerEntry
	)
	paginator := s3.NewListObjectVersionsPaginator(client, &s3.ListObjectVersionsInput{
		Bucket: aws.String(src.bucket),
		Prefix: aws.String(src.prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, trace.Wrap(err, "failed to list object versions")
		}
		versions = append(versions, page.Versions...)
		deleteMarkers = append(deleteMarkers, page.DeleteMarkers...)
	}

	return selectObjectsAsOf(versions, deleteMarkers, src.prefix, asOf), nil
}

// selectObjectsAsOf reconstructs the set of objects that were current at asOf. For each key it keeps the
// newest entry (object version or delete marker) whose LastModified is at or before asOf; a delete marker
// means the object did not exist at asOf and is omitted. Entries written after asOf are ignored, so the
// result is deterministic even though the listing is not an atomic snapshot (asOf is a fixed past instant
// and versions are immutable). On an exact LastModified tie a version beats a delete marker (treated as
// present), since versions are considered first.
//
// asOf (the orchestrator's wall clock) is compared directly against S3's LastModified; the clocks are
// assumed NTP-synchronized, as significant skew would shift selection around the consistency point.
func selectObjectsAsOf(versions []types.ObjectVersion, deleteMarkers []types.DeleteMarkerEntry, prefix string, asOf time.Time) []remoteObject {
	type candidate struct {
		lastModified time.Time
		deleted      bool
		size         int64
		versionID    *string
	}

	current := make(map[string]candidate, len(versions))
	consider := func(key string, lastModified time.Time, c candidate) {
		if lastModified.After(asOf) {
			return // written after the consistency point
		}

		if existing, ok := current[key]; ok && !lastModified.After(existing.lastModified) {
			return // an at-least-as-recent entry already won this key
		}

		current[key] = c
	}

	for _, version := range versions {
		lastModified := aws.ToTime(version.LastModified)
		consider(aws.ToString(version.Key), lastModified, candidate{
			lastModified: lastModified,
			size:         aws.ToInt64(version.Size),
			versionID:    version.VersionId,
		})
	}

	for _, deleteMarker := range deleteMarkers {
		lastModified := aws.ToTime(deleteMarker.LastModified)
		consider(aws.ToString(deleteMarker.Key), lastModified, candidate{
			lastModified: lastModified,
			deleted:      true,
		})
	}

	objects := make([]remoteObject, 0, len(current))
	for key, candidate := range current {
		if candidate.deleted {
			continue
		}

		rel := relativeKey(prefix, key)
		if rel == "" {
			continue
		}

		objects = append(objects, remoteObject{
			key:          key,
			relPath:      rel,
			versionID:    candidate.versionID,
			size:         candidate.size,
			lastModified: candidate.lastModified,
		})
	}

	return objects
}

// relativeKey returns the object key relative to the prefix, or "" for keys that should be skipped
// (directory-placeholder objects, or a key equal to the prefix).
func relativeKey(prefix, key string) string {
	if strings.HasSuffix(key, "/") {
		return "" // "directory" placeholder object
	}

	rel := strings.TrimPrefix(key, prefix)
	rel = strings.TrimPrefix(rel, "/")
	return rel
}

// removeExtraneousLocalFiles deletes files under destDir whose relative path is not in keep.
func removeExtraneousLocalFiles(destDir string, keep map[string]struct{}) error {
	info, err := os.Stat(destDir)
	if os.IsNotExist(err) {
		return nil
	}

	if err != nil {
		return trace.Wrap(err, "failed to stat %q", destDir)
	}

	if !info.IsDir() {
		return nil
	}

	return filepath.WalkDir(destDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return trace.Wrap(walkErr, "failed to walk %q", p)
		}

		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(destDir, p)
		if err != nil {
			return trace.Wrap(err, "failed to compute path of %q relative to %q", p, destDir)
		}

		if _, ok := keep[rel]; ok {
			return nil
		}

		if err := os.Remove(p); err != nil {
			return trace.Wrap(err, "failed to remove stale file %q", p)
		}

		return nil
	})
}

// localFile is a regular file discovered under the upload source directory.
type localFile struct {
	relPath string // slash-separated path relative to the base directory
	absPath string
	size    int64
	modTime time.Time
}

// listLocalFiles walks baseDir and returns its regular files. A non-existent directory yields no files
// (so an empty/absent source uploads nothing rather than erroring).
func listLocalFiles(baseDir string) ([]localFile, error) {
	info, err := os.Stat(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, trace.Wrap(err, "failed to stat %q", baseDir)
	}

	if !info.IsDir() {
		return nil, trace.Errorf("source path %q is not a directory", baseDir)
	}

	var files []localFile
	err = filepath.WalkDir(baseDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return trace.Wrap(walkErr, "failed to walk %q", p)
		}

		// Skip directories and special files (symlinks, sockets, devices); only regular files are objects.
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}

		fi, err := d.Info()
		if err != nil {
			return trace.Wrap(err, "failed to stat %q", p)
		}

		rel, err := filepath.Rel(baseDir, p)
		if err != nil {
			return trace.Wrap(err, "failed to compute path of %q relative to %q", p, baseDir)
		}

		files = append(files, localFile{
			relPath: filepath.ToSlash(rel),
			absPath: p,
			size:    fi.Size(),
			modTime: fi.ModTime(),
		})

		return nil
	})

	return files, trace.Wrap(err, "failed to walk %q", baseDir)
}

// detectContentType guesses an object's content type from its file extension, falling back to sniffing the
// first bytes of content. It always returns a non-empty type (http.DetectContentType defaults to
// application/octet-stream). The file offset is reset to the start before returning.
func detectContentType(f *os.File, path string) (string, error) {
	if contentType := mime.TypeByExtension(filepath.Ext(path)); contentType != "" {
		return contentType, nil
	}

	buffer := make([]byte, 512)
	n, err := f.Read(buffer)
	if err != nil && err != io.EOF {
		return "", trace.Wrap(err, "failed to read %q for content type detection", path)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", trace.Wrap(err, "failed to seek %q back to start", path)
	}

	return http.DetectContentType(buffer[:n]), nil
}

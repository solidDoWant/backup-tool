package s3

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// injectClient points the runtime's client factory at the given mock.
func injectClient(rt *LocalRuntime, client s3API) {
	rt.newS3Client = func(_ aws.Config, _ ...func(*awss3.Options)) s3API {
		return client
	}
}

func objectVersion(key, versionID string, size int64, lastModified time.Time) types.ObjectVersion {
	return types.ObjectVersion{
		Key:          aws.String(key),
		VersionId:    aws.String(versionID),
		Size:         aws.Int64(size),
		LastModified: aws.Time(lastModified),
	}
}

func deleteMarker(key string, lastModified time.Time) types.DeleteMarkerEntry {
	return types.DeleteMarkerEntry{
		Key:          aws.String(key),
		LastModified: aws.Time(lastModified),
	}
}

func TestSelectObjectsAsOf(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	asOf := base.Add(10 * time.Minute)

	t.Run("picks the newest version at or before asOf", func(t *testing.T) {
		objects := selectObjectsAsOf([]types.ObjectVersion{
			objectVersion("a", "old", 1, base),
			objectVersion("a", "current", 2, base.Add(5*time.Minute)),
			objectVersion("a", "future", 3, asOf.Add(time.Minute)),
		}, nil, "", asOf)

		require.Len(t, objects, 1)
		assert.Equal(t, "current", aws.ToString(objects[0].versionID))
		assert.EqualValues(t, 2, objects[0].size)
	})

	t.Run("excludes objects whose only versions are after asOf", func(t *testing.T) {
		objects := selectObjectsAsOf([]types.ObjectVersion{
			objectVersion("a", "future", 1, asOf.Add(time.Minute)),
		}, nil, "", asOf)
		assert.Empty(t, objects)
	})

	t.Run("omits an object deleted at or before asOf", func(t *testing.T) {
		objects := selectObjectsAsOf([]types.ObjectVersion{
			objectVersion("a", "v1", 1, base),
		}, []types.DeleteMarkerEntry{
			deleteMarker("a", base.Add(5*time.Minute)),
		}, "", asOf)
		assert.Empty(t, objects)
	})

	t.Run("keeps an object recreated after a delete but before asOf", func(t *testing.T) {
		objects := selectObjectsAsOf([]types.ObjectVersion{
			objectVersion("a", "v1", 1, base),
			objectVersion("a", "v2", 9, base.Add(8*time.Minute)),
		}, []types.DeleteMarkerEntry{
			deleteMarker("a", base.Add(5*time.Minute)),
		}, "", asOf)

		require.Len(t, objects, 1)
		assert.Equal(t, "v2", aws.ToString(objects[0].versionID))
	})

	t.Run("ignores a delete marker that is after asOf", func(t *testing.T) {
		objects := selectObjectsAsOf([]types.ObjectVersion{
			objectVersion("a", "v1", 1, base),
		}, []types.DeleteMarkerEntry{
			deleteMarker("a", asOf.Add(time.Minute)),
		}, "", asOf)

		require.Len(t, objects, 1)
		assert.Equal(t, "v1", aws.ToString(objects[0].versionID))
	})

	t.Run("relativizes keys against the prefix and skips directory placeholders", func(t *testing.T) {
		objects := selectObjectsAsOf([]types.ObjectVersion{
			objectVersion("media/sub/a.txt", "v1", 1, base),
			objectVersion("media/", "vdir", 0, base), // directory placeholder
		}, nil, "media", asOf)

		require.Len(t, objects, 1)
		assert.Equal(t, "sub/a.txt", objects[0].relPath)
	})
}

func TestSyncRejectsUnsupportedDirections(t *testing.T) {
	rt := NewLocalRuntime()
	injectClient(rt, NewMocks3API(t))
	creds := NewCredentials("id", "secret")

	assert.Error(t, rt.Sync(th.NewTestContext(), creds, "s3://a/x", "s3://b/y", time.Time{}))
	assert.Error(t, rt.Sync(th.NewTestContext(), creds, "/local/x", "/local/y", time.Time{}))
}

func TestSyncDownloadLatestState(t *testing.T) {
	destDir := t.TempDir()
	// A pre-existing file with no source counterpart must be pruned.
	require.NoError(t, os.WriteFile(filepath.Join(destDir, "stale.txt"), []byte("old"), 0o644))

	client := NewMocks3API(t)
	client.EXPECT().ListObjectsV2(mock.Anything, mock.Anything, mock.Anything).Return(&awss3.ListObjectsV2Output{
		Contents: []types.Object{{
			Key:          aws.String("prefix/a.txt"),
			Size:         aws.Int64(3),
			LastModified: aws.Time(time.Now()),
		}},
	}, nil)
	client.EXPECT().GetObject(mock.Anything, mock.MatchedBy(func(in *awss3.GetObjectInput) bool {
		return aws.ToString(in.Key) == "prefix/a.txt" && in.VersionId == nil
	})).Return(&awss3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("abc"))}, nil)

	rt := NewLocalRuntime()
	injectClient(rt, client)

	// Zero asOf => latest-state sync; versioning is never queried.
	err := rt.Sync(th.NewTestContext(), NewCredentials("id", "secret"), "s3://bucket/prefix", destDir, time.Time{})
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(destDir, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "abc", string(got))
	assert.NoFileExists(t, filepath.Join(destDir, "stale.txt"))
}

func TestSyncDownloadPointInTime(t *testing.T) {
	destDir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	asOf := base.Add(30 * time.Minute)

	client := NewMocks3API(t)
	client.EXPECT().GetBucketVersioning(mock.Anything, mock.Anything).
		Return(&awss3.GetBucketVersioningOutput{Status: types.BucketVersioningStatusEnabled}, nil)
	client.EXPECT().ListObjectVersions(mock.Anything, mock.Anything, mock.Anything).Return(&awss3.ListObjectVersionsOutput{
		Versions: []types.ObjectVersion{
			objectVersion("prefix/keep.txt", "v2", 2, base.Add(10*time.Minute)),
			objectVersion("prefix/keep.txt", "v1", 1, base),
			objectVersion("prefix/after.txt", "vfuture", 5, asOf.Add(time.Minute)),
			objectVersion("prefix/deleted.txt", "vd", 4, base),
		},
		DeleteMarkers: []types.DeleteMarkerEntry{
			deleteMarker("prefix/deleted.txt", base.Add(5*time.Minute)),
		},
	}, nil)
	// Only keep.txt@v2 should be fetched, and by its specific version.
	client.EXPECT().GetObject(mock.Anything, mock.MatchedBy(func(in *awss3.GetObjectInput) bool {
		return aws.ToString(in.Key) == "prefix/keep.txt" && aws.ToString(in.VersionId) == "v2"
	})).Return(&awss3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("keep"))}, nil)

	rt := NewLocalRuntime()
	injectClient(rt, client)

	err := rt.Sync(th.NewTestContext(), NewCredentials("id", "secret"), "s3://bucket/prefix", destDir, asOf)
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(destDir, "keep.txt"))
	require.NoError(t, err)
	assert.Equal(t, "keep", string(got))
	assert.NoFileExists(t, filepath.Join(destDir, "after.txt"))
	assert.NoFileExists(t, filepath.Join(destDir, "deleted.txt"))
}

func TestSyncDownloadFallsBackWhenVersioningDisabled(t *testing.T) {
	destDir := t.TempDir()

	client := NewMocks3API(t)
	client.EXPECT().GetBucketVersioning(mock.Anything, mock.Anything).
		Return(&awss3.GetBucketVersioningOutput{Status: types.BucketVersioningStatusSuspended}, nil)
	// Falls back to a latest-state sync: ListObjectVersions must not be used.
	client.EXPECT().ListObjectsV2(mock.Anything, mock.Anything, mock.Anything).Return(&awss3.ListObjectsV2Output{
		Contents: []types.Object{{
			Key:          aws.String("prefix/a.txt"),
			Size:         aws.Int64(3),
			LastModified: aws.Time(time.Now()),
		}},
	}, nil)
	client.EXPECT().GetObject(mock.Anything, mock.Anything).
		Return(&awss3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("abc"))}, nil)

	rt := NewLocalRuntime()
	injectClient(rt, client)

	err := rt.Sync(th.NewTestContext(), NewCredentials("id", "secret"), "s3://bucket/prefix", destDir, time.Now())
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(destDir, "a.txt"))
}

func TestSyncDownloadFallsBackWhenVersioningCheckErrors(t *testing.T) {
	destDir := t.TempDir()

	client := NewMocks3API(t)
	// Versioning status can't be determined (e.g. unsupported API / missing permission).
	client.EXPECT().GetBucketVersioning(mock.Anything, mock.Anything).Return(nil, assert.AnError)
	// It must degrade to a latest-state sync rather than fail.
	client.EXPECT().ListObjectsV2(mock.Anything, mock.Anything, mock.Anything).Return(&awss3.ListObjectsV2Output{
		Contents: []types.Object{{
			Key:          aws.String("prefix/a.txt"),
			Size:         aws.Int64(3),
			LastModified: aws.Time(time.Now()),
		}},
	}, nil)
	client.EXPECT().GetObject(mock.Anything, mock.Anything).
		Return(&awss3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("abc"))}, nil)

	rt := NewLocalRuntime()
	injectClient(rt, client)

	err := rt.Sync(th.NewTestContext(), NewCredentials("id", "secret"), "s3://bucket/prefix", destDir, time.Now())
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(destDir, "a.txt"))
}

func TestSyncUpload(t *testing.T) {
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("abc"), 0o644))

	client := NewMocks3API(t)
	// Remote currently holds an object with no local counterpart; it must be deleted.
	client.EXPECT().ListObjectsV2(mock.Anything, mock.Anything, mock.Anything).Return(&awss3.ListObjectsV2Output{
		Contents: []types.Object{{
			Key:          aws.String("prefix/stale.txt"),
			Size:         aws.Int64(9),
			LastModified: aws.Time(time.Now()),
		}},
	}, nil)
	client.EXPECT().PutObject(mock.Anything, mock.MatchedBy(func(in *awss3.PutObjectInput) bool {
		return aws.ToString(in.Key) == "prefix/a.txt"
	})).Return(&awss3.PutObjectOutput{}, nil)
	client.EXPECT().DeleteObject(mock.Anything, mock.MatchedBy(func(in *awss3.DeleteObjectInput) bool {
		return aws.ToString(in.Key) == "prefix/stale.txt"
	})).Return(&awss3.DeleteObjectOutput{}, nil)

	rt := NewLocalRuntime()
	injectClient(rt, client)

	err := rt.Sync(th.NewTestContext(), NewCredentials("id", "secret"), srcDir, "s3://bucket/prefix", time.Time{})
	require.NoError(t, err)
}

func TestSyncDownloadPropagatesListError(t *testing.T) {
	client := NewMocks3API(t)
	client.EXPECT().ListObjectsV2(mock.Anything, mock.Anything, mock.Anything).Return(nil, assert.AnError)

	rt := NewLocalRuntime()
	injectClient(rt, client)

	err := rt.Sync(th.NewTestContext(), NewCredentials("id", "secret"), "s3://bucket/prefix", t.TempDir(), time.Time{})
	assert.Error(t, err)
}

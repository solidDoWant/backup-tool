# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`backup-tool` is a Go CLI for performing consistent disaster-recovery (DR) backups and restores of Kubernetes-hosted applications. Three apps are hand-written Go assemblers (Vaultwarden, Teleport, Authentik); a fourth, `generic`, is **config-driven** — a standard app (N volumes / M CNPG clusters / O S3 buckets) is expressed as a declarative YAML config instead of new Go, reusing the same engine (see `pkg/disasterrecovery/generic.go`). It runs either locally (driving a remote cluster) or in-cluster (as a transient pod spawned by the local invocation). The same binary serves three roles, switched by subcommand:

- `dr <app> <backup|restore> run --config-file <yaml>` — drives a DR event from outside the cluster.
- `dr <app> <backup|restore> gen-config-schema` — emits a JSON schema for the YAML config.
- `grpc` — the in-cluster pod mode; serves files/postgres/s3 RPCs to the driver on TCP port 40983 (plaintext). The driver reaches it directly via the pod IP, so no Service is created and the driver must be on the cluster pod network.

Backups produce on-cluster PVC snapshots (no external storage hop), and DBs are dumped logically (via `pg_dumpall`) so they're human-readable and don't require an identical target. See `docs/design decisions.md` for the rationale.

## Common commands

All canonical workflows go through the Makefile:

```
make build                  # local-arch binary + tarball + container image + helm package
make binary                 # local-arch Go binary only -> build/binaries/<os>/<arch>/backup-tool
make container-image        # local-arch container image (depends on binary + licenses)
make helm                   # package the dr-job Helm chart
make build-all              # all platforms (linux/amd64, linux/arm64) + multi-arch manifest
make test                   # `go test -timeout 30s -failfast -v ./cmd/... ./pkg/...` (unit tests only; e2e is excluded — run those with `go test ./e2e/...`)
make generate-all           # run every code generator (protobuf, mocks, CNPG client, barman-cloud client, approver-policy client, DR schemas)
make generate-protobuf-code # regenerate pkg/grpc/gen from .proto files
make generate-mocks         # run mockery (config in .mockery.yaml)
make generate-barman-cloud-client # regenerate the barman-cloud plugin ObjectStore clientset
make generate-dr-schemas    # regenerate schemas/<app>-<event>.schema.json (requires building the binary)
make clean                  # rm build/, working/, deps charts/, drop local container image
make clean-e2e              # tear down leaked e2e resources (kind clusters, registries, zpool, loop devices)
```

Single-test runs use `go test` directly:
```
go test -run TestFoo ./pkg/disasterrecovery/...
go test -v -count=1 ./pkg/grpc/clients/
```

E2e tests under `e2e/dr/` are heavy: they spin up a kind cluster, push the image to a local registry, install dependent services via helmfile, and create a real ZFS zpool on a loop device. Run with `go test ./e2e/...`. If a run fails mid-setup, `make clean-e2e` cleans up leaked containers/zpool/loop devices.

The release flow is `make release PUSH_ALL=true VERSION=x.y.z` (defaults to a no-op echo when `PUSH_ALL` is unset).

## Architecture

### Command layer (`cmd/`)
- `cmd/root.go` wires `dr`, `grpc`, and `version` onto the root cobra command.
- `cmd/disasterrecovery/drcommand.go` enumerates registered DR apps via the `DRCommand` interface; each app implements `DRBackupCommand` / `DRRestoreCommand` to expose `backup` / `restore` subcommands.
- `cmd/disasterrecovery/clusterdrcommand.go` is the generic glue (`ClusterDRCommand[TBackupConfig, TRestoreConfig]`): a per-app config struct + a `func(ctx, config, kubeCluster) error` is enough to plug a new app in. The shared command handles flags, config loading, validation, and schema generation.
- Per-app `*.go` (e.g. `vaultwarden.go`) defines the strongly-typed config (which embeds the pkg-level `*Options` struct) and supplies the run callback. Required fields are tagged `jsonschema:"required"`.
- `cmd/disasterrecovery/generic.go` wires the **config-driven** `generic` app: its config types live entirely in `pkg/disasterrecovery/generic.go` (no cmd-level wrapper to embed), so the run callback just hands the parsed config to `GenericApp.Backup`/`.Restore`.

### DR orchestration (`pkg/disasterrecovery/`)
A high-level Backup/Restore method (e.g. `VaultWarden.Backup`) is a thin assembler: it ensures the DR PVC exists (`NewDRVolume`), builds a `RemoteStage` from a list of `RemoteAction`s, runs it, and snapshots the DR PVC. The stage does the heavy lifting:
1. Snapshot/clone PVCs and clone CNPG clusters via composite operations (inside each action).
2. Spawn a single `backup-tool` pod (`backuptoolinstance`) inside the target namespace with every action's volumes/secrets mounted.
3. Connect to that pod over gRPC (plaintext, directly to the pod IP — no Service) and drive file/Postgres/S3 work remotely. Because it dials the pod IP, the driver must run on the cluster pod network (i.e. in-cluster, as the dr-job Job does).
4. Snapshot the resulting DR PVC.
5. Tear everything down via deferred `cleanup.To(...)`.

**All apps use the `RemoteStage` pattern** (`pkg/disasterrecovery/actions/remote`): `RemoteStage.WithAction(...).Run(ctx)` validates → sets up → executes a list of `RemoteAction`s against a single tool-pod lifecycle. **Use it for new applications too.** Each app composes the available actions: `cnpg/backup` + `cnpg/restore` (Postgres logical dump/restore of a cloned/target cluster), `files/backup` + `files/restore` (a data-directory PVC captured into / restored from a subdirectory of the DR volume — Vaultwarden's data dir), `files/groupbackup` + `files/grouprestore` (a **label-selected group of PVCs** captured atomically as one `VolumeGroupSnapshot` into / restored from `fileGroups/<group>/<pvc>` subtrees of the DR volume — used by the `generic` app's `fileGroups` sources; the shared on-disk layout constant lives in `actions/remote/files/layout.FileGroupsDirName`), and `s3sync` (object-store captures — Teleport audit logs, Authentik media). Adding a new data source means writing a new `RemoteAction`, not a new imperative script.

The `generic` app (`pkg/disasterrecovery/generic.go`, `GenericApp.Backup`/`.Restore`) is the config-driven path over this same machinery: a `GenericBackupConfig`/`GenericRestoreConfig` lists sources grouped by kind (`postgres`/`files`/`fileGroups`/`s3`), and the app registers the matching actions in a fixed, consistency-correct kind order (postgres → files → fileGroups → s3) regardless of file order — so a config, not new Go, onboards a standard app. The on-disk dump filename is derived from each postgres source's slot `name` (`<name>.sql`); files/s3 use the slot `name` as their DR-volume subdirectory; a `fileGroups` source captures its label-selected members under `fileGroups/<name>/<pvc>` (one subdir per member PVC, `<name>` being the group slot). A `fileGroups` source is keyed by a `selector` (a `metav1.LabelSelector`, required non-empty) supplied identically at backup and restore — restore re-resolves the live target PVCs from it rather than reading an on-disk manifest, and refuses to start unless the captured members map 1:1 onto the resolved targets. S3 credentials are an optional inline `s3.Credentials` (else AWS env vars). v1 scope: single namespace, in-place restore against a **pre-existing** DR PVC (no snapshot hydration; the restore reads the DR PVC named `backupName`, materialized out-of-band from a backup snapshot when the source is gone), no persisted-config write.

### Kubernetes client (`pkg/kubecluster/`)
- `client.go` exposes a single `ClientInterface` that embeds smaller clients. Composites are wired to their primitive deps in `NewClient`.
- `primatives/` — thin wrappers, **one per upstream clientset / client library** (not per API group). So a single primitive spans every group its clientset serves: `core` (the standard `k8s.io/client-go/kubernetes` clientset, hence Pods/PVCs/Secrets/Services from `core/v1` **plus** Jobs from `batch/v1` and EndpointSlices from `discovery/v1`); `cnpg` (CloudNative-PG clusters); `certmanager`; `externalsnapshotter` (the external-snapshotter `versioned` clientset, covering **both** the `snapshot.storage.k8s.io` group — `VolumeSnapshot…` — and the `groupsnapshot.storage.k8s.io` group — `VolumeGroupSnapshot…`); `approverpolicy`; `barmancloud` (the barman-cloud plugin's `ObjectStore` resources; read-only `GetObjectStore`). Each exposes a `ClientInterface`. Primitives stay thin — no multi-step orchestration or deferred cleanup (`cleanup.To` lives only in composites).
- `composite/` — multi-resource operations that orchestrate primitives (and the place for any `cleanup.To` lifecycle work): `clonepvc` (clone a single PVC via a `VolumeSnapshot`, or a label-selected group atomically via a `VolumeGroupSnapshot` — `ClonePVC` / `ClonePVCGroup` on one provider, sharing internal create-from-snapshot and force-bind helpers), `clonedcluster` (CNPG PITR clone with TLS), `clusterusercert` (issue + CRP allow), `createcrpforcertificate`, `drvolume` (DR PVC + CNPG ImageCatalog), `backuptoolinstance` (the in-cluster pod).
- `helpers/` — `MaxWaitTime`, watcher utilities, `FullName`, naming helpers (CNPG enforces a 40-char limit on cloned-cluster names).

The CNPG, barman-cloud, and approver-policy primitives are partially generated (`make generate-cnpg-client`, `make generate-barman-cloud-client`, `make generate-approver-policy-client`). The CNPG and barman-cloud generators pin to the versions recorded in `go.mod` (the barman-cloud generator also rewrites the plugin's controller-runtime-style `GroupVersion` to the `SchemeGroupVersion` that client-gen expects — see the Makefile target); approver-policy still pins to `main` until upstream cuts a release with the needed fix (see Makefile comments).

**Barman WAL archiving — plugin only.** Source clusters archive WAL via the barman-cloud CNPG-I plugin (`.spec.plugins` referencing a `barmancloud.cnpg.io/v1` `ObjectStore`). CNPG's deprecated in-tree barman WAL archiving (`.spec.backup.barmanObjectStore`) is **not supported** — `clonedcluster.configureWALRecovery` errors if the source cluster has no barman-cloud WAL-archiver plugin (`findBarmanCloudWALArchiver` returns nil). The clone recovers via `bootstrap.recovery.source` + `volumeSnapshots`, fetching WAL through an `externalClusters[].plugin` reference to the same `ObjectStore` (read-only — the clone never archives). This is the path that honors a wall-clock `recoveryTarget.targetTime`, so true PITR works for every source.

**Recovery target & cross-resource PITR.** Cloning a cluster is split into `CreateClusterBackup` (take the base backup — the consistency point) and `CloneClusterFromBackup` (create the recovering clone); there is no monolithic `CloneCluster` (it was removed once every caller adopted the split). A caller that wants the DB aligned with other DR captures takes the base backup **first** (fixing the consistency point before the filesystem/S3 captures), then creates the clone recovering forward to `T_dr` (the last non-DB capture instant). When a `RecoveryTargetTime` is supplied the clone recovers via `recoveryTarget.targetTime`. To make that forward recovery reach the target even when the source committed nothing after it (an idle source would otherwise have no WAL at/after the target), a **source WAL recovery fence** is written before the clone (see below). `CloneClusterFromBackup` keeps a `targetImmediate` **fallback** (the consistency point — data-identical for an idle DB) only as a backstop for the rare case the fence can't be established.

Failure detection **watches the recovery Job to its terminal condition** (no polling, no gate, no log parsing). CNPG runs recovery in a Job whose retries (`backoffLimit` window) last long enough to wait out the source's `archive_timeout`, so the Job **completes** once an attempt reaches the target — the clone then comes up — and **fails** only if it exhausts retries without reaching the target, which yields `ErrRecoveryTargetNotReached` and the fallback. The source recovery fence (below) makes the target reachable even on an idle source, so this failure path is now a rare backstop rather than the idle-source norm. `waitForCloneRecovery` watches that Job via a label-selected watch (`cnpg.io/cluster=<clone>,cnpg.io/jobRole`, using `core.WaitForJobCompletion` / `helpers.WaitForResourceConditionByLabel`); `Complete` → wait for the cluster to be ready, `Failed` → fall back. This is chosen over the cluster status (CNPG surfaces no condition for "recovery ran out of WAL" — it flattens the `recovery ended before configured recovery target was reached` FATAL to a generic exit-status-1 error) and over a one-shot pod-phase check (a snapshot can't tell an idle retry from a slow replay). The terminal verdict survives CNPG garbage-collecting a completed Job, since the delete watch event still carries the Job's final state. When no `RecoveryTargetTime` is supplied, the clone recovers to the consistency point via `targetImmediate` and is awaited with a plain `WaitForReadyCluster`. Every app drives this split flow through the RemoteStage `cnpg/backup` action; for Vaultwarden the shared consistency point is its data-directory PVC clone time, so the DB recovers forward to exactly the moment the data dir was frozen (see the RemoteStage consistency-point protocol below).

**Source WAL recovery fence — make forward recovery reach the target on any source.** There's no config knob and no path branching (every source uses the barman-cloud plugin). After the recovery target is fixed and before the clone, `backup.ForceSourceWALArchive` → `postgres.ForceWALArchive` writes a **recovery fence** on the source: a no-op committing transaction (`SELECT txid_current()` forces XID assignment, so the commit writes an `XLOG_XACT_COMMIT` record — a transaction that touches no data and is never assigned an XID writes none), then `pg_switch_wal()` and a wait for the segment to archive (`pg_stat_archiver`). One fence fixes both idle-source failures at once:

1. **The consistency-point segment gets archived.** An idle source can leave the base backup's consistency point on a fresh WAL segment boundary that is never written into (and so never archived); even a `targetImmediate` recovery then can't fetch that segment and fails with `recovery ended before configured recovery target was reached`. The fence's commit record lands in that segment and the switch completes and archives it.

2. **Forward recovery can reach the target.** `recovery_target_time` only ever stops at a transaction commit, **never at a `pg_create_restore_point`** (verified from a captured forward-recovery Job log — a restore point timestamped *after* the target was applied without stopping). The fence is a commit timestamped *after* the recovery target, so the clone's forward recovery confirms it replayed past the target and stops cleanly even when the source committed nothing since the base backup. The clone therefore **always** recovers forward (`targetTime`) — no idle special-case, no doomed forward attempt that exhausts the recovery Job's retries.

This **replaced** an earlier LSN-comparison idle-detection scheme (capture `pg_current_wal_lsn()` before and after the target; if unchanged, clear `RecoveryTargetTime` and recover via `targetImmediate`). That was timing-fragile: any WAL between the pre-capture and the target — post-migration autovacuum, a stray background commit — read as "active", so a source that was actually idle *past* the target still skipped the short-circuit and paid the full ~16 min forward-recovery-then-fallback cost. The fence is robust because it makes the forward recovery itself succeed instead of trying to predict when to avoid it; `CloneClusterFromBackup`'s `targetImmediate` fallback is kept only as a now-rarely-hit backstop. The exec primitive is `core.ExecInPod` (client-go SPDY `remotecommand`, container `postgres`, local socket `-h /controller/run`, in-pod superuser — no certs/Service/pod-network); the fence orchestration is `postgres.ForceWALArchive(ctx, run PSQLRunner, …)`, decoupled from how psql runs. Requires the dr-job ServiceAccount to hold **`pods/exec` (create)** (granted by the chart). It runs for every source, written by the RemoteStage `cnpg/backup` action's `Setup` for every app.

**RemoteStage consistency-point protocol.** Cross-resource alignment is driven by the **`RemoteStage`** rather than the app: the stage's setup runs in three phases (`pkg/disasterrecovery/actions/remote/remotestage.go`), coordinated by two optional, type-asserted action capabilities. In phase 1 it calls `BeforeConsistencyPoint` on every action implementing `PreConsistencyPointAction`, **which returns a `time.Time`** — the instant that action's capture pinned, or the zero time if it pins none. Two kinds of pre-step work: a **base backup** (the `cnpg/backup` action) takes each cluster's base backup before any clone exists and pins nothing (returns zero — it only needs to *precede* the point); a **filesystem freeze** (the `files/backup` action) clones a live data-directory PVC and returns the clone's creation time, because a volume snapshot exists only at the moment it is taken and cannot be reconstructed for an arbitrary instant. The stage then sets the shared consistency point `C` to the **earliest non-zero instant any pre-step pinned**, falling back to `time.Now()` when none pinned one (the Teleport/Authentik case — only base backups run). Choosing the earliest keeps every forward-recoverable capture (a cluster clone) and as-of capture (S3) from landing later than an already-frozen filesystem. In phase 2 it hands `C` to every action implementing `ConsistencyPointConsumer` (`SetConsistencyPoint`). In phase 3 it runs each action's `Setup`, where the `cnpg/backup` action writes the source recovery fence and clones via `CloneClusterFromBackup`, recovering **forward to `C`** (the fence makes `C` reachable even for a cluster idle since its base backup; the `targetImmediate` fallback remains only as a backstop), and the `files/backup` action mounts its already-taken clone. The CNPG action owns its base backup end to end: takes it in `BeforeConsistencyPoint`, clones from it in `Setup`, deletes it in `Cleanup` (which tolerates partial state, and runs at the end of the event so the backup outlives clone creation, since the clone's recovery snapshots are owned by it); the files action likewise owns its PVC clone (taken in `BeforeConsistencyPoint`, deleted in `Cleanup`).

**Ordering constraint:** a base backup must be registered (and so run) before any filesystem freeze whose instant it must precede — the clone recovers *forward* from a base backup taken earlier, so a base backup taken after the freeze that defines `C` could not reach `C`. Vaultwarden therefore registers `cnpg/backup` before `files/backup`. With this, `C` for Vaultwarden = its data-directory clone time, and the DB recovers forward to exactly that instant — reproducing the original (pre-RemoteStage) Vaultwarden behaviour where the DB was aligned to the data-PVC clone. For Teleport/Authentik nothing pins an instant, so `C = time.Now()` after the base backups and the databases land mutually aligned to it (the S3 captures are taken as of `C` too). `C` is the orchestrator's wall clock, not the Postgres/WAL clock (the clock-reconciliation caveat applies).

The S3 captures (Teleport audit session logs, Authentik media) are rewound to `C` too. The S3 sync action implements `ConsistencyPointConsumer`, so it receives `C` and threads it to the tool pod as the `Sync` request's `as_of` timestamp (a download passes `C`; an upload/restore passes zero — restore re-uploads the already-as-of-`C` directory). The pod's `Sync` handler (`LocalRuntime.Sync` in `pkg/s3/transfer.go`) honors `as_of`: on a **download** with a non-zero `as_of`, if the source bucket has versioning **enabled** it reconstructs the bucket as of `C` — `ListObjectVersions` (versions + delete markers), per key pick the newest entry with `LastModified ≤ C` (a delete marker means the object didn't exist → omit it), then `GetObject` that specific `VersionId` — and prunes local files absent as of `C`. When versioning is **off/suspended** (or `as_of` is zero, e.g. uploads) it falls back to a latest-state sync and **warns** that the event isn't guaranteed cross-resource consistent. `C` (orchestrator wall clock) is compared **directly** against S3's `LastModified` — clocks are assumed NTP-synced (the clock-skew caveat). The S3 layer uses **aws-sdk-go-v2 directly** (the `seqsense/s3sync` dependency and aws-sdk-go v1 were dropped); the `pkg/s3` test seam is the `s3API` interface plus the `selectObjectsAsOf` pure function. Point-in-time capture requires the operator-supplied S3 credentials to allow `s3:GetBucketVersioning`, `s3:ListBucketVersions`, and `s3:GetObjectVersion` (no chart change — these creds are user-provided). So an event's databases **and** S3 captures all align to `C`.

All three e2e apps (vaultwarden, teleport, authentik) configure their source/restore clusters to archive WAL via the barman-cloud plugin. The dr-job chart's RBAC grants read-only access to `objectstores.barmancloud.cnpg.io` (harmless when the CRD is absent).

### gRPC (`pkg/grpc/`)
- `proto/backup-tool/{files,postgres,s3}/v1/*.proto` — sources. Generated code lives in `gen/`, including a `*_grpc_mock.pb.go` testify mock from `protoc-gen-go-grpcmock`.
- `servers/` — handlers running inside the spawned pod. Each registers via `registerServers` in `server.go` along with the gRPC standard healthcheck. The server uses two interceptors that propagate the long-lived process `*contexts.Context` into per-request handler contexts via `WrapHandlerContext` / `UnwrapHandlerContext`.
- `clients/` — typed wrappers used by the local driver (`Files()`, `Postgres()`, `S3()`). The files service includes `ListDirectory(path)` (immediate-subdirectory names only, non-recursive); the group restore action uses it to enumerate captured `fileGroups/<group>/<pvc>` members and enforce the 1:1 capture↔target check before syncing.

### Cross-cutting

**`pkg/contexts/Context`** wraps `context.Context` and adds a `Log` (charmbracelet) and `Stopwatch`. Almost every internal API takes `*contexts.Context`, and `ctx.Child()` is called when delegating to a sub-step (resets stopwatch, indents logs). When inside a gRPC handler, call `contexts.UnwrapHandlerContext(stdCtx)` to recover it.

**`pkg/cleanup.To(fn).WithErrMessage(...).WithOriginalErr(&err).WithParentCtx(ctx).WithTimeout(...).Run()`** is the standard deferred-cleanup pattern. It runs on `context.Background` so cleanup still executes if the parent was cancelled, and aggregates its error into the named return via `trace.NewAggregate`. Pair it with named return values (`func ... (foo *Bar, err error)`) so `&err` works.

**Errors** use `github.com/gravitational/trace` everywhere — wrap with `trace.Wrap(err, "msg %q", arg)` rather than `fmt.Errorf`. The root command prints `trace.DebugReport` for full stacks.

**Config loading** (`pkg/cli/features/configfile.go`): YAML is parsed with `goccy/go-yaml` (strict mode), validated with `go-playground/validator`, and JSON schemas are emitted via `invopop/jsonschema`. The `jsonschema:"required"` tag is automatically translated to a `validate:"required"` tag at runtime, so authors only tag once. Per `docs/design decisions.md`: required fields go on the receiver or as explicit args; optional fields go in a non-nil `Opts` struct (that's why every `Backup`/`Restore` has trailing `*Options` parameters).

### Mocks

Mocks are checked-in (`*_mock.go`, naming convention `<interface>_mock.go` lowercased — see `.mockery.yaml`). Regenerate with `make generate-mocks` after changing any interface listed there. To add a new mocked interface, edit `.mockery.yaml`.

### Deployable artifacts

- Container: `Dockerfile` (multi-arch, expects `build/binaries/$TARGETOS/$TARGETARCH/backup-tool` to exist; built by `make container-manifest`).
- Helm chart: `deploy/charts/dr-job` — packaged by `make helm` as `build/helm/dr-job-$VERSION.tgz`. The chart name is intentionally generic; both backup and restore are run as one-shot Jobs.
- JSON schemas in `schemas/` are committed and regenerated by `make generate-dr-schemas` (which runs the locally-built binary).

## Conventions worth knowing

- Volume mount paths and on-disk filenames inside DR PVCs (e.g. `vaultwardenDRVolPath = "data-vol"`, `vaultwardenSQLFileName = "dump.sql"`, and the `generic` app's `fileGroups/<group>/<pvc>` layout rooted at `actions/remote/files/layout.FileGroupsDirName`) are part of the on-disk format. Changing them breaks restore of older backups — the comments call this out and the constraint still holds.
- Cloned CNPG cluster names are capped at 40 chars (CNPG limit); `helpers.CleanName` + truncation logic in `vaultwarden.go` is the established pattern.
- `dario.cat/mergo.Merge` is used to layer user-supplied `RemoteBackupToolOptions` over computed `CreateBackupToolInstanceOptions`; preserve this order when adding new options.
- Postgres connections to cloned CNPG clusters use TLS client-cert auth (`require_auth=none`, `sslmode=verify-full`); credentials are file paths into the pod's mounted secret volumes, not env-var values for passwords.

## Maintaining this file

Update CLAUDE.md when a change would make a section above misleading to a future agent. Concrete triggers:

- A new DR app is added (update the Overview app list and the DR orchestration section if it uses a new pattern).
- A Makefile target is added/renamed/removed, or the canonical build/test/generate flow changes.
- A new top-level package appears under `pkg/`, or a package's role changes materially.
- The gRPC port, proto layout, or in-cluster pod contract changes.
- An on-disk DR-volume path/filename or other backup-format constant is added (these are restore-compatibility load-bearing).
- A cross-cutting convention shifts (context/cleanup/error/config-tag patterns, mock generation setup).

Skip updates for routine bug fixes, refactors that don't change the architecture described here, and dependency bumps.

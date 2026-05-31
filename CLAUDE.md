# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`backup-tool` is a Go CLI for performing consistent disaster-recovery (DR) backups and restores of Kubernetes-hosted applications (currently Vaultwarden, Teleport, Authentik). It runs either locally (driving a remote cluster) or in-cluster (as a transient pod spawned by the local invocation). The same binary serves three roles, switched by subcommand:

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

### DR orchestration (`pkg/disasterrecovery/`)
The high-level Backup/Restore methods (e.g. `VaultWarden.Backup`) are imperative scripts that:
1. Snapshot/clone PVCs and clone CNPG clusters via composite operations.
2. Spawn a `backup-tool` pod (`backuptoolinstance`) inside the target namespace with the relevant volumes/secrets mounted.
3. Connect to that pod over gRPC (plaintext, directly to the pod IP — no Service) and drive file/Postgres/S3 work remotely. Because it dials the pod IP, the driver must run on the cluster pod network (i.e. in-cluster, as the dr-job Job does).
4. Snapshot the resulting DR PVC.
5. Tear everything down via deferred `cleanup.To(...)`.

Newer apps (Authentik) build the same flow declaratively using `pkg/disasterrecovery/actions/remote`: `RemoteStage.WithAction(...).Run(ctx)` validates → sets up → executes a list of `RemoteAction`s (cnpg backup/restore, s3sync, etc.) against a single tool-pod lifecycle. **Prefer the RemoteStage pattern for new applications.**

### Kubernetes client (`pkg/kubecluster/`)
- `client.go` exposes a single `ClientInterface` that embeds smaller clients. Composites are wired to their primitive deps in `NewClient`.
- `primatives/` — thin wrappers over typed clients per CRD group: `core` (PVCs/Pods/Secrets/Services), `cnpg` (CloudNative-PG clusters), `certmanager`, `externalsnapshotter`, `approverpolicy`, `barmancloud` (the barman-cloud plugin's `ObjectStore` resources; read-only `GetObjectStore`). Each exposes a `ClientInterface`.
- `composite/` — multi-resource operations that orchestrate primitives: `clonepvc`, `clonedcluster` (CNPG PITR clone with TLS), `clusterusercert` (issue + CRP allow), `createcrpforcertificate`, `drvolume` (DR PVC + CNPG ImageCatalog), `backuptoolinstance` (the in-cluster pod).
- `helpers/` — `MaxWaitTime`, watcher utilities, `FullName`, naming helpers (CNPG enforces a 40-char limit on cloned-cluster names).

The CNPG, barman-cloud, and approver-policy primitives are partially generated (`make generate-cnpg-client`, `make generate-barman-cloud-client`, `make generate-approver-policy-client`). The CNPG and barman-cloud generators pin to the versions recorded in `go.mod` (the barman-cloud generator also rewrites the plugin's controller-runtime-style `GroupVersion` to the `SchemeGroupVersion` that client-gen expects — see the Makefile target); approver-policy still pins to `main` until upstream cuts a release with the needed fix (see Makefile comments).

**Barman WAL archiving — in-tree vs plugin.** CNPG deprecated in-tree barman WAL archiving (`.spec.backup.barmanObjectStore`) in favor of the barman-cloud CNPG-I plugin (`.spec.plugins` referencing a `barmancloud.cnpg.io/v1` `ObjectStore`). `clonedcluster.CloneCluster` supports both, and the cloned cluster recovers to the same point in both: the backup's **consistency point** (volume-snapshot base + WAL replayed from the source's object store only up to that point). It inspects the source cluster; for plugin-based sources it recovers via `bootstrap.recovery.source` + `volumeSnapshots`, fetching WAL through an `externalClusters[].plugin` reference to the same `ObjectStore` (read-only — the clone never archives), and **must** set `recoveryTarget.targetImmediate` so recovery stops at the consistency point. For in-tree sources it recovers from the `Backup` object directly (`recovery.backup`); CNPG's snapshot recovery for a Backup object recovers to the consistency point and **ignores** `recoveryTarget.targetTime`, so the deprecated path never did true wall-clock PITR either — the `targetImmediate` plugin path matches it. (Don't "fix" the plugin path to honor a wall-clock `recoveryTarget.targetTime`: on a quiescent source it fails with `recovery ended before configured recovery target was reached`, since no transaction commits at/after the target.) In-tree-specific option fields/branches are marked `// Deprecated:` but remain fully supported. The vaultwarden e2e exercises the plugin path; teleport/authentik stay on in-tree for coverage. The dr-job chart's RBAC grants read-only access to `objectstores.barmancloud.cnpg.io` (harmless when the CRD is absent).

### gRPC (`pkg/grpc/`)
- `proto/backup-tool/{files,postgres,s3}/v1/*.proto` — sources. Generated code lives in `gen/`, including a `*_grpc_mock.pb.go` testify mock from `protoc-gen-go-grpcmock`.
- `servers/` — handlers running inside the spawned pod. Each registers via `registerServers` in `server.go` along with the gRPC standard healthcheck. The server uses two interceptors that propagate the long-lived process `*contexts.Context` into per-request handler contexts via `WrapHandlerContext` / `UnwrapHandlerContext`.
- `clients/` — typed wrappers used by the local driver (`Files()`, `Postgres()`, `S3()`).

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

- Volume mount paths and on-disk filenames inside DR PVCs (e.g. `vaultwardenDRVolPath = "data-vol"`, `vaultwardenSQLFileName = "dump.sql"`) are part of the on-disk format. Changing them breaks restore of older backups — the comments call this out and the constraint still holds.
- Cloned CNPG cluster names are capped at 40 chars (CNPG limit); `helpers.CleanName` + truncation logic in `vaultwarden.go` is the established pattern.
- `dario.cat/mergo.MergeWithOverwrite` is used to layer user-supplied `RemoteBackupToolOptions` over computed `CreateBackupToolInstanceOptions`; preserve this order when adding new options.
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

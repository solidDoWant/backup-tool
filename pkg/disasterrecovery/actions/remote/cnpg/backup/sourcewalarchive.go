package backup

import (
	cnpgpostgres "github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
	cnpgspecs "github.com/cloudnative-pg/cloudnative-pg/pkg/specs"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/composite/clonedcluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/postgres"
)

// sourcePSQLRunner returns a postgres.PSQLRunner that runs each statement by exec'ing psql in the
// source cluster's primary pod over the pod's local Postgres socket (authenticating as the in-pod
// superuser), so it needs no certificates, Service, or pod-network reachability — only that the
// dr-job's ServiceAccount may exec into pods. -h points at the local socket dir for trust auth; -tA
// returns the bare result; ON_ERROR_STOP makes a failed statement a non-zero exit.
func sourcePSQLRunner(ctx *contexts.Context, kubeClient kubecluster.ClientInterface, namespace, sourceClusterName string) (postgres.PSQLRunner, error) {
	cluster, err := kubeClient.CNPG().GetCluster(ctx.Child(), namespace, sourceClusterName)
	if err != nil {
		return nil, trace.Wrap(err, "failed to get source cluster %q", helpers.FullNameStr(namespace, sourceClusterName))
	}

	// Wall-clock PITR — and therefore the forced WAL archive — only applies when the source archives WAL
	// through the barman-cloud plugin. In-tree (or non-archiving) sources never recover forward to a
	// wall-clock target, so there is nothing to force.
	if !clonedcluster.UsesBarmanCloudWALArchiver(cluster) {
		return nil, nil
	}

	primaryPod := cluster.Status.CurrentPrimary
	if primaryPod == "" {
		return nil, trace.Errorf("source cluster %q has no current primary to exec into", helpers.FullNameStr(namespace, sourceClusterName))
	}

	run := func(ctx *contexts.Context, sql string) (string, error) {
		command := []string{"psql", "-h", cnpgpostgres.SocketDirectory, "-X", "-q", "-t", "-A", "-w", "-v", "ON_ERROR_STOP=1", "-c", sql}
		stdout, _, execErr := kubeClient.Core().ExecInPod(ctx, namespace, primaryPod, cnpgspecs.PostgresContainerName, command, nil)
		return stdout, trace.Wrap(execErr, "failed to run psql statement on source primary %q", helpers.FullNameStr(namespace, primaryPod))
	}
	return run, nil
}

// ForceSourceWALArchive writes a WAL recovery fence on a barman-cloud-plugin (PITR) source and forces
// its segment to archive — see postgres.ForceWALArchive for the mechanism and why a committing fence
// (not a restore point) is required. It must run after the recovery target is fixed and before the
// clone, so the fence's commit timestamp lands past the target and the WAL is archived by the time
// recovery needs it.
//
// For in-tree (or non-archiving) sources it is a no-op: those never recover forward to a wall-clock
// target. Shared by every PITR-cloning app (Vaultwarden directly, and the RemoteStage CNPG backup
// action for Teleport/Authentik/future apps).
func ForceSourceWALArchive(ctx *contexts.Context, kubeClient kubecluster.ClientInterface, namespace, sourceClusterName string) (err error) {
	ctx.Log.With("sourceCluster", sourceClusterName).Info("Forcing WAL archive on source cluster")
	defer ctx.Log.Info("Finished forcing WAL archive on source cluster", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	run, err := sourcePSQLRunner(ctx, kubeClient, namespace, sourceClusterName)
	if err != nil {
		return err
	}

	if run == nil {
		ctx.Log.Debug("Source does not use the barman-cloud plugin; skipping WAL archive")
		return nil
	}

	return postgres.ForceWALArchive(ctx.Child(), run, postgres.ForceWALArchiveOptions{})
}

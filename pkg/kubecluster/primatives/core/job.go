package core

import (
	"errors"
	"slices"
	"time"

	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

// ErrJobFailed is returned by WaitForJobCompletion when the Job reaches a Failed terminal state. It is
// distinct from a timeout (the Job never finished) so callers can tell the two apart. It is a plain
// sentinel (not trace.Errorf) so that errors.Is still matches it through a trace.Wrap.
var ErrJobFailed = errors.New("job failed")

type WaitForJobCompletionOpts struct {
	helpers.MaxWaitTime
	// LabelSelector selects the Job by label instead of by name, for when the Job's name isn't known
	// ahead of time (e.g. CNPG's generate-named recovery Jobs). Pass a name argument or set this, not
	// both; when set, the name argument is ignored.
	LabelSelector string
}

// WaitForJobCompletion watches (rather than polls) for a Job to complete, returning it once it does.
// If the Job reaches a Failed terminal state instead, it returns ErrJobFailed; if it never finishes,
// it returns a timeout error. The Job is selected by name, or by opts.LabelSelector when its name
// isn't known ahead of time. A failed Job is reported even if it is subsequently garbage collected,
// since the delete watch event still carries the Job's final state.
func (c *Client) WaitForJobCompletion(ctx *contexts.Context, namespace, name string, opts WaitForJobCompletionOpts) (job *batchv1.Job, err error) {
	ctx.Log.With("name", name, "labelSelector", opts.LabelSelector).Info("Waiting for job to complete")
	defer ctx.Log.Info("Finished waiting for job", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	processEvent := func(_ *contexts.Context, job *batchv1.Job) (*batchv1.Job, bool, error) {
		if jobHasTrueCondition(job, batchv1.JobComplete) {
			return job, true, nil
		}

		if jobHasTrueCondition(job, batchv1.JobFailed) {
			return nil, true, trace.Wrap(ErrJobFailed, "job %q failed", helpers.FullNameStr(namespace, name))
		}

		return nil, false, nil
	}

	jobs := c.client.BatchV1().Jobs(namespace)
	timeout := opts.MaxWait(15 * time.Minute)
	if opts.LabelSelector != "" {
		job, err = helpers.WaitForResourceConditionByLabel(ctx.Child(), timeout, jobs, opts.LabelSelector, processEvent)
	} else {
		job, err = helpers.WaitForResourceCondition(ctx.Child(), timeout, jobs, name, processEvent)
	}

	return job, trace.Wrap(err, "failed waiting for job %q to complete", helpers.FullNameStr(namespace, name))
}

func jobHasTrueCondition(job *batchv1.Job, condType batchv1.JobConditionType) bool {
	return slices.ContainsFunc(job.Status.Conditions, func(c batchv1.JobCondition) bool {
		return c.Type == condType && c.Status == corev1.ConditionTrue
	})
}

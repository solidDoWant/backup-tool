package core

import (
	"errors"
	"testing"
	"time"

	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWaitForJobCompletion(t *testing.T) {
	namespace := "test-ns"
	labelSelector := "cnpg.io/jobRole"

	jobWith := func(condType batchv1.JobConditionType) *batchv1.Job {
		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "recovery",
				Namespace: namespace,
				Labels:    map[string]string{"cnpg.io/jobRole": "snapshot-recovery"},
			},
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{{Type: condType, Status: corev1.ConditionTrue}},
			},
		}
	}

	tests := []struct {
		desc          string
		job           *batchv1.Job
		wantErr       bool // expect any error (no successful completion)
		wantFailedErr bool // expect ErrJobFailed specifically
	}{
		{desc: "job complete returns the job", job: jobWith(batchv1.JobComplete)},
		{desc: "job failed returns ErrJobFailed", job: jobWith(batchv1.JobFailed), wantErr: true, wantFailedErr: true},
		{desc: "no terminal job times out", job: nil, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			c, mockK8s := createTestClient()
			ctx := th.NewTestContext()

			if tt.job != nil {
				_, err := mockK8s.BatchV1().Jobs(namespace).Create(ctx, tt.job, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			job, err := c.WaitForJobCompletion(ctx, namespace, "", WaitForJobCompletionOpts{MaxWaitTime: helpers.MaxWaitTime(time.Second), LabelSelector: labelSelector})
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, job)
				assert.Equal(t, tt.wantFailedErr, errors.Is(err, ErrJobFailed))
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, job)
		})
	}
}

func TestWaitForJobCompletionByName(t *testing.T) {
	namespace := "test-ns"
	c, mockK8s := createTestClient()
	ctx := th.NewTestContext()

	_, err := mockK8s.BatchV1().Jobs(namespace).Create(ctx, &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "named-job", Namespace: namespace},
		Status:     batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	job, err := c.WaitForJobCompletion(ctx, namespace, "named-job", WaitForJobCompletionOpts{MaxWaitTime: helpers.MaxWaitTime(time.Second)})
	require.NoError(t, err)
	assert.NotNil(t, job)
}

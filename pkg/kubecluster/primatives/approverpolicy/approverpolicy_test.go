package approverpolicy

import (
	"sync"
	"testing"
	"time"

	policyv1alpha1 "github.com/cert-manager/approver-policy/pkg/apis/policy/v1alpha1"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/approverpolicy/gen/clientset/versioned"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubetesting "k8s.io/client-go/testing"
)

func TestCreateCertificateRequestPolicy(t *testing.T) {
	standardCRPSpec := policyv1alpha1.CertificateRequestPolicySpec{}

	tests := []struct {
		desc                  string
		spec                  policyv1alpha1.CertificateRequestPolicySpec
		opts                  CreateCertificateRequestPolicyOptions
		expectedPolicy        *policyv1alpha1.CertificateRequestPolicy
		simulateClientFailure bool
	}{
		{
			desc: "no options",
			spec: standardCRPSpec,
			opts: CreateCertificateRequestPolicyOptions{},
			expectedPolicy: &policyv1alpha1.CertificateRequestPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-policy",
				},
				Spec: standardCRPSpec,
			},
		},
		{
			desc: "all options",
			spec: standardCRPSpec,
			opts: CreateCertificateRequestPolicyOptions{
				GenerateName: true,
			},
			expectedPolicy: &policyv1alpha1.CertificateRequestPolicy{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-policy",
				},
				Spec: standardCRPSpec,
			},
		},
		{
			desc:                  "simulate client failure",
			simulateClientFailure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			client, fakeClientset := createTestClient()
			if tt.simulateClientFailure {
				fakeClientset.PrependReactor("create", "certificaterequestpolicies", func(action kubetesting.Action) (bool, runtime.Object, error) {
					return true, nil, assert.AnError
				})
			}

			ctx := th.NewTestContext()
			policy, err := client.CreateCertificateRequestPolicy(ctx, "test-policy", tt.spec, tt.opts)

			if tt.simulateClientFailure {
				require.Error(t, err)
				assert.Nil(t, policy)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, policy)
			assert.Equal(t, tt.expectedPolicy, policy)
		})
	}
}

func TestWaitForReadyCertificateRequestPolicy(t *testing.T) {
	crpName := "test-crp"

	noConditionsCRP := &policyv1alpha1.CertificateRequestPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: crpName,
		},
	}

	notReadyCRP := noConditionsCRP.DeepCopy()
	notReadyCondition := policyv1alpha1.CertificateRequestPolicyCondition{Type: policyv1alpha1.CertificateRequestPolicyConditionReady, Status: corev1.ConditionFalse}
	notReadyCRP.Status.Conditions = append(notReadyCRP.Status.Conditions, notReadyCondition)

	readyCRP := notReadyCRP.DeepCopy()
	readyCondition := notReadyCondition.DeepCopy()
	readyCondition.Status = corev1.ConditionTrue
	readyCRP.Status.Conditions[0] = *readyCondition

	multipleConditionsCRP := readyCRP.DeepCopy()
	// CRP does not have multiple conditions (yet)
	dummyCondition := policyv1alpha1.CertificateRequestPolicyCondition{Type: "DummyCondition", Status: corev1.ConditionFalse}
	multipleConditionsCRP.Status.Conditions = []policyv1alpha1.CertificateRequestPolicyCondition{dummyCondition, *readyCondition}

	tests := []struct {
		desc                string
		initialCRP          *policyv1alpha1.CertificateRequestPolicy
		shouldError         bool
		afterStartedWaiting func(*testing.T, *contexts.Context, versioned.Interface)
	}{
		{
			desc:       "CRP starts ready",
			initialCRP: readyCRP,
		},
		{
			desc:        "CRP not ready",
			initialCRP:  notReadyCRP,
			shouldError: true,
		},
		{
			desc:        "CRP does not exist",
			shouldError: true,
		},
		{
			desc:       "CRP becomes ready",
			initialCRP: notReadyCRP,
			afterStartedWaiting: func(t *testing.T, ctx *contexts.Context, client versioned.Interface) {
				_, err := client.PolicyV1alpha1().CertificateRequestPolicies().Update(ctx, readyCRP, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
		{
			desc:       "multiple conditions",
			initialCRP: notReadyCRP,
			afterStartedWaiting: func(t *testing.T, ctx *contexts.Context, client versioned.Interface) {
				_, err := client.PolicyV1alpha1().CertificateRequestPolicies().Update(ctx, multipleConditionsCRP, metav1.UpdateOptions{})
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			client, fakeClientset := createTestClient()
			ctx := th.NewTestContext()

			if tt.initialCRP != nil {
				_, err := fakeClientset.PolicyV1alpha1().CertificateRequestPolicies().Create(ctx, tt.initialCRP, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			var wg sync.WaitGroup
			var waitErr error
			var crp *policyv1alpha1.CertificateRequestPolicy
			wg.Add(1)
			go func() {
				crp, waitErr = client.WaitForReadyCertificateRequestPolicy(ctx, crpName, WaitForReadyCertificateRequestPolicyOpts{MaxWaitTime: helpers.ShortWaitTime})
				wg.Done()
			}()

			if tt.afterStartedWaiting != nil {
				time.Sleep(10 * time.Millisecond) // Ensure that watcher has been setup
				tt.afterStartedWaiting(t, ctx, fakeClientset)
			}

			wg.Wait()
			if tt.shouldError {
				assert.Error(t, waitErr)
				assert.Nil(t, crp)
				return
			}
			assert.NoError(t, waitErr)
			assert.NotNil(t, crp)
		})
	}
}

func TestDeleteCertificateRequestPolicy(t *testing.T) {
	crpName := "test-crp"

	tests := []struct {
		desc           string
		shouldSetupCRP bool
		wantErr        bool
	}{
		{
			desc:           "successful delete",
			shouldSetupCRP: true,
		},
		{
			desc:    "delete non-existent CRP",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			client, _ := createTestClient()
			ctx := th.NewTestContext()

			var existingCRP *policyv1alpha1.CertificateRequestPolicy
			if tt.shouldSetupCRP {
				existingCRP = &policyv1alpha1.CertificateRequestPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: crpName,
					},
				}
				_, err := client.client.PolicyV1alpha1().CertificateRequestPolicies().Create(ctx, existingCRP, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			err := client.DeleteCertificateRequestPolicy(ctx, crpName)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

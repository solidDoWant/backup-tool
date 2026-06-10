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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubetesting "k8s.io/client-go/testing"
)

func TestIsAvailable(t *testing.T) {
	gv := policyv1alpha1.SchemeGroupVersion.String()

	tests := []struct {
		desc          string
		resources     []*metav1.APIResourceList
		reactorErr    bool
		wantAvailable bool
		wantErr       bool
	}{
		{
			desc: "approver-policy installed",
			resources: []*metav1.APIResourceList{{
				GroupVersion: gv,
				APIResources: []metav1.APIResource{{Name: "certificaterequestpolicies", Kind: "CertificateRequestPolicy"}},
			}},
			wantAvailable: true,
		},
		{
			desc: "group present without the CRP kind",
			resources: []*metav1.APIResourceList{{
				GroupVersion: gv,
				APIResources: []metav1.APIResource{{Name: "somethingelses", Kind: "SomethingElse"}},
			}},
			wantAvailable: false,
		},
		{
			// The fake discovery returns a NotFound error when the group version is absent, matching a
			// real API server with approver-policy uninstalled. This must be treated as "not available",
			// not as an error.
			desc:          "approver-policy not installed",
			resources:     nil,
			wantAvailable: false,
		},
		{
			desc:       "discovery error",
			reactorErr: true,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			client, fakeClientset := createTestClient()
			fakeClientset.Fake.Resources = tt.resources
			if tt.reactorErr {
				fakeClientset.PrependReactor("get", "resource", func(action kubetesting.Action) (bool, runtime.Object, error) {
					return true, nil, assert.AnError
				})
			}

			ctx := th.NewTestContext()
			available, err := client.IsAvailable(ctx)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantAvailable, available)
		})
	}
}

func TestIsAvailableMemoized(t *testing.T) {
	client, fakeClientset := createTestClient()
	fakeClientset.Fake.Resources = []*metav1.APIResourceList{{
		GroupVersion: policyv1alpha1.SchemeGroupVersion.String(),
		APIResources: []metav1.APIResource{{Name: "certificaterequestpolicies", Kind: "CertificateRequestPolicy"}},
	}}
	ctx := th.NewTestContext()

	available, err := client.IsAvailable(ctx)
	require.NoError(t, err)
	assert.True(t, available)

	// Remove the API. A non-memoized implementation would now report false; the cached result must hold.
	fakeClientset.Fake.Resources = nil
	available, err = client.IsAvailable(ctx)
	require.NoError(t, err)
	assert.True(t, available)
}

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
	notReadyCondition := metav1.Condition{Type: policyv1alpha1.ConditionTypeReady, Status: metav1.ConditionFalse}
	notReadyCRP.Status.Conditions = append(notReadyCRP.Status.Conditions, notReadyCondition)

	readyCRP := notReadyCRP.DeepCopy()
	readyCondition := notReadyCondition.DeepCopy()
	readyCondition.Status = metav1.ConditionTrue
	readyCRP.Status.Conditions[0] = *readyCondition

	multipleConditionsCRP := readyCRP.DeepCopy()
	// CRP does not have multiple conditions (yet)
	dummyCondition := metav1.Condition{Type: "DummyCondition", Status: metav1.ConditionFalse}
	multipleConditionsCRP.Status.Conditions = []metav1.Condition{dummyCondition, *readyCondition}

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

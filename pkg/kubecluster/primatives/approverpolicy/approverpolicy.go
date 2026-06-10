package approverpolicy

import (
	"slices"
	"time"

	policyv1alpha1 "github.com/cert-manager/approver-policy/pkg/apis/policy/v1alpha1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IsAvailable reports whether the cert-manager approver-policy API is installed on the cluster (i.e.
// the CertificateRequestPolicy CRD is registered). This is the cluster-wide "all or nothing" signal for
// CertificateRequestPolicies: when approver-policy is installed it disables cert-manager's built-in
// CertificateRequest approver and takes over approval, so every certificate this tool issues needs a
// matching CertificateRequestPolicy to be approved; when it is absent the built-in approver handles
// approval and creating a policy would fail against a non-existent API. The result is memoized for the
// life of the client, as the set of installed cluster APIs does not change within a single DR event.
func (c *Client) IsAvailable(ctx *contexts.Context) (bool, error) {
	c.availableMu.Lock()
	defer c.availableMu.Unlock()

	if c.availableCache != nil {
		return *c.availableCache, nil
	}

	gv := policyv1alpha1.SchemeGroupVersion
	ctx.Log.With("groupVersion", gv.String()).Debug("Checking whether the approver-policy API is installed")

	resources, err := c.client.Discovery().ServerResourcesForGroupVersion(gv.String())
	if err != nil && !apierrors.IsNotFound(err) {
		return false, trace.Wrap(err, "failed to query the API server for the %q API group", gv.String())
	}

	available := resources != nil && slices.ContainsFunc(resources.APIResources, func(resource metav1.APIResource) bool {
		return resource.Kind == policyv1alpha1.CertificateRequestPolicyKind
	})

	c.availableCache = &available
	return available, nil
}

type CreateCertificateRequestPolicyOptions struct {
	helpers.GenerateName
}

func (c *Client) CreateCertificateRequestPolicy(ctx *contexts.Context, name string, spec policyv1alpha1.CertificateRequestPolicySpec, opts CreateCertificateRequestPolicyOptions) (*policyv1alpha1.CertificateRequestPolicy, error) {
	ctx.Log.With("name", name).Info("Creating certificate request policy")
	ctx.Log.Debug("Call parameters", "spec", spec, "opts", opts)

	policy := &policyv1alpha1.CertificateRequestPolicy{
		Spec: spec,
	}

	opts.SetName(&policy.ObjectMeta, name)

	policy, err := c.client.PolicyV1alpha1().CertificateRequestPolicies().Create(ctx, policy, metav1.CreateOptions{})
	if err != nil {
		return nil, trace.Wrap(err, "failed to create certificate request policy %q", name)
	}

	return policy, nil
}

type WaitForReadyCertificateRequestPolicyOpts struct {
	helpers.MaxWaitTime
}

func (c *Client) WaitForReadyCertificateRequestPolicy(ctx *contexts.Context, name string, opts WaitForReadyCertificateRequestPolicyOpts) (policy *policyv1alpha1.CertificateRequestPolicy, err error) {
	ctx.Log.With("name", name).Info("Waiting for certificate request policy to become ready")
	defer ctx.Log.Info("Finished waiting for certificate request policy to become ready", ctx.Stopwatch.Keyval(), contexts.ErrorKeyvals(&err))

	precondition := func(ctx *contexts.Context, policy *policyv1alpha1.CertificateRequestPolicy) (*policyv1alpha1.CertificateRequestPolicy, bool, error) {
		ctx.Log.Debug("Policy conditions", "conditions", policy.Status.Conditions)
		isReady := false
		for _, condition := range policy.Status.Conditions {
			if condition.Type != policyv1alpha1.ConditionTypeReady {
				continue
			}

			// Upstream uses corev1 package, see https://github.com/solidDoWant/approver-policy/blob/20e3371bd325ecb8c9dbb9600720fb81969ae11a/pkg/internal/controllers/certificaterequestpolicies.go#L244
			isReady = condition.Status == metav1.ConditionTrue
			break
		}

		if isReady {
			return policy, true, nil
		}

		return nil, false, nil
	}
	policy, err = helpers.WaitForResourceCondition(ctx.Child(), opts.MaxWait(time.Minute), c.client.PolicyV1alpha1().CertificateRequestPolicies(), name, precondition)
	if err != nil {
		return nil, trace.Wrap(err, "failed waiting for certificate request policy %q to become ready", name)
	}

	return policy, nil
}

func (c *Client) DeleteCertificateRequestPolicy(ctx *contexts.Context, name string) error {
	ctx.Log.With("name", name).Info("Deleting certificate request policy")

	err := c.client.PolicyV1alpha1().CertificateRequestPolicies().Delete(ctx.Child(), name, metav1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete certificate request policy %q", name)
}

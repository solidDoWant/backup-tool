package approverpolicy

import (
	"time"

	policyv1alpha1 "github.com/cert-manager/approver-policy/pkg/apis/policy/v1alpha1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/helpers"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

	policy, err := c.client.PolicyV1alpha1().CertificateRequestPolicies().Create(ctx, policy, v1.CreateOptions{})
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
			if condition.Type != policyv1alpha1.CertificateRequestPolicyConditionReady {
				continue
			}

			// Upstream uses corev1 package, see https://github.com/solidDoWant/approver-policy/blob/20e3371bd325ecb8c9dbb9600720fb81969ae11a/pkg/internal/controllers/certificaterequestpolicies.go#L244
			isReady = condition.Status == corev1.ConditionTrue
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

	err := c.client.PolicyV1alpha1().CertificateRequestPolicies().Delete(ctx.Child(), name, v1.DeleteOptions{})
	return trace.Wrap(err, "failed to delete certificate request policy %q", name)
}

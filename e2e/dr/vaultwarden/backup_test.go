package vaultwarden

import (
	"context"
	"testing"

	"github.com/gravitational/trace"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/pkg/utils"
)

func TestBack(t *testing.T) {
	// Applying the resource will succeed but the pod will fail to start because the image
	// is not being replaced. This will be removed later - for now it's just a placeholder.
	f1 := features.New("test deploying pod").
		WithLabel("type", "setup").
		Assess("pod can deploy", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			p := utils.RunCommand("kubectl apply -f templates/registry.yaml")
			assert.NoError(t, trace.Wrap(p.Err(), "failed to deploy pod: %s", p.Result()))
			return ctx
		}).
		Feature()

	testenv.Test(t, f1)
}

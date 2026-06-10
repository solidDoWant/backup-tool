package common

import (
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
)

// Validates that the issuer exists, whether it is a cluster issuer or a namespaced issuer.
func ValidateIssuer(ctx *contexts.Context, kubeClusterClient kubecluster.ClientInterface, namespace string, issuer cmmeta.IssuerReference) error {
	issuerKind := issuer.Kind
	if issuerKind == "" {
		issuerKind = "Issuer" // Default to "Issuer" (namespace-specific), like cert-manger
	}

	var issuerStatus *certmanagerv1.IssuerStatus
	switch issuerKind {
	case "Issuer":
		namespacedIssuer, err := kubeClusterClient.CM().GetIssuer(ctx.Child(), namespace, issuer.Name)
		if err != nil {
			return trace.Wrap(err, "failed to get CNPG cluster cert issuer %q", issuer.Name)
		}
		issuerStatus = &namespacedIssuer.Status
	case "ClusterIssuer":
		clusterIssuer, err := kubeClusterClient.CM().GetClusterIssuer(ctx.Child(), issuer.Name)
		if err != nil {
			return trace.Wrap(err, "failed to get CNPG cluster cert cluster issuer %q", issuer.Name)
		}
		issuerStatus = &clusterIssuer.Status
	default:
		return trace.Errorf("unknown issuer kind %q", issuerKind)
	}

	if !certmanager.IsIssuerReady(issuerStatus) {
		return trace.Errorf("CNPG cluster cert issuer %q is not ready", issuer.Name)
	}

	return nil
}

package common

import (
	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/gravitational/trace"
	"github.com/solidDoWant/backup-tool/pkg/contexts"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
)

// Validates that the issuer exists, whether it is a cluster issuer or a namespaced issuer.
func ValidateIssuer(ctx *contexts.Context, kubeClusterClient kubecluster.ClientInterface, namespace, issuerKind, issuerName string) error {
	if issuerKind == "" {
		issuerKind = "Issuer" // Default to "Issuer" (namespace-specific), like cert-manger
	}

	var issuerStatus *certmanagerv1.IssuerStatus
	switch issuerKind {
	case "Issuer":
		issuer, err := kubeClusterClient.CM().GetIssuer(ctx.Child(), namespace, issuerName)
		if err != nil {
			return trace.Wrap(err, "failed to get CNPG cluster cert issuer %q", issuerName)
		}
		issuerStatus = &issuer.Status
	case "ClusterIssuer":
		issuer, err := kubeClusterClient.CM().GetClusterIssuer(ctx.Child(), issuerName)
		if err != nil {
			return trace.Wrap(err, "failed to get CNPG cluster cert cluster issuer %q", issuerName)
		}
		issuerStatus = &issuer.Status
	default:
		return trace.Errorf("unknown issuer kind %q", issuerKind)
	}

	if !certmanager.IsIssuerReady(issuerStatus) {
		return trace.Errorf("CNPG cluster cert issuer %q is not ready", issuerName)
	}

	return nil
}

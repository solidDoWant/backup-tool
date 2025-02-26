package common

import (
	"testing"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster"
	"github.com/solidDoWant/backup-tool/pkg/kubecluster/primatives/certmanager"
	th "github.com/solidDoWant/backup-tool/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestValidateIssuer(t *testing.T) {
	namespace := "test-namespace"
	issuerName := "test-issuer"

	issuerTypes := map[string]func(client *certmanager.MockClientInterface, status certmanagerv1.IssuerStatus) bool{
		"Issuer": func(client *certmanager.MockClientInterface, status certmanagerv1.IssuerStatus) bool {
			client.EXPECT().GetIssuer(mock.Anything, namespace, issuerName).Return(&certmanagerv1.Issuer{
				Status: status,
			}, nil)

			return false
		},
		"ClusterIssuer": func(client *certmanager.MockClientInterface, status certmanagerv1.IssuerStatus) bool {
			client.EXPECT().GetClusterIssuer(mock.Anything, issuerName).Return(&certmanagerv1.ClusterIssuer{
				Status: status,
			}, nil)

			return false
		},
		"Invalid": func(client *certmanager.MockClientInterface, status certmanagerv1.IssuerStatus) bool {
			return true
		},
		"": func(client *certmanager.MockClientInterface, status certmanagerv1.IssuerStatus) bool {
			client.EXPECT().GetIssuer(mock.Anything, namespace, issuerName).Return(&certmanagerv1.Issuer{
				Status: status,
			}, nil)

			return false
		},
	}
	issuerReadyConditions := []cmmeta.ConditionStatus{cmmeta.ConditionTrue, cmmeta.ConditionFalse}

	for issuerType, issuerSetup := range issuerTypes {
		for _, issuerReadyCondition := range issuerReadyConditions {
			t.Run(issuerType+"-"+string(issuerReadyCondition), func(t *testing.T) {
				ctx := th.NewTestContext()
				cmClient := certmanager.NewMockClientInterface(t)
				kubeClusterClient := kubecluster.NewMockClientInterface(t)
				kubeClusterClient.EXPECT().CM().Return(cmClient).Maybe()

				status := certmanagerv1.IssuerStatus{
					Conditions: []certmanagerv1.IssuerCondition{
						{
							Type:   certmanagerv1.IssuerConditionReady,
							Status: issuerReadyCondition,
						},
					},
				}
				wantErr := issuerSetup(cmClient, status)

				err := ValidateIssuer(ctx, kubeClusterClient, namespace, issuerType, issuerName)

				if issuerReadyCondition == cmmeta.ConditionTrue && !wantErr {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
			})
		}
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package orchestrator

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenariokindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	awskindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

//go:embed agent_api_key_values.yaml
var agentAPIKeyRefreshValuesFmt string

// TestZzzClusterAgentAPIKeyRefresh tests the agent's ability to refresh the API key
// Zzz is used to ensure this test runs last in the suite as it requires redeploying the agent
func (suite *k8sSuite) TestZzzClusterAgentAPIKeyRefresh() {
	namespace := "datadog"
	secretName := "apikeyrefresh"
	apiKeyOld := "abcdefghijklmnopqrstuvwxyz123456"

	// apply secret containing the old API key which is used by the agent
	suite.applySecret(namespace, secretName, map[string][]byte{"apikey": []byte(apiKeyOld)})

	// install the agent with old API key
	suite.UpdateEnv(
		awskindvm.Provisioner(
			awskindvm.WithRunOptions(
				scenariokindvm.WithAgentOptions(
					kubernetesagentparams.WithNamespace(namespace),
					kubernetesagentparams.WithHelmValues(fmt.Sprintf(agentAPIKeyRefreshValuesFmt, suite.Env().FakeIntake.URL)),
				),
			),
		),
	)

	// verify that the old API key exists in the orchestrator resources payloads
	suite.eventuallyHasExpectedAPIKey(apiKeyOld)

	// update the secret with a new API key and agent will refresh it
	apiKeyNew := "123456abcdefghijklmnopqrstuvwxyz"
	suite.applySecret(namespace, secretName, map[string][]byte{"apikey": []byte(apiKeyNew)})

	// verify that the new API key exists in the orchestrator resources payloads
	suite.eventuallyHasExpectedAPIKey(apiKeyNew)
}

// applySecret creates or updates a secret in the given namespace with the provided data.
func (suite *k8sSuite) applySecret(namespace, name string, secretData map[string][]byte) {
	client := suite.Env().KubernetesCluster.KubernetesClient.K8sClient

	// Check if the namespace exists, create it if not
	if _, err := client.CoreV1().Namespaces().Get(context.Background(), namespace, v1.GetOptions{}); err != nil {
		_, err = client.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
			ObjectMeta: v1.ObjectMeta{
				Name: namespace,
			},
		}, v1.CreateOptions{})
		require.NoError(suite.T(), err)
	}

	secret := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: secretData,
	}

	// Check if the secret already exists, update it if it does
	var err error
	if _, err = client.CoreV1().Secrets(namespace).Get(context.Background(), name, v1.GetOptions{}); err != nil {
		_, err = client.CoreV1().Secrets(namespace).Create(context.Background(), secret, v1.CreateOptions{})
	} else {
		_, err = client.CoreV1().Secrets(namespace).Update(context.Background(), secret, v1.UpdateOptions{})
	}
	require.NoError(suite.T(), err)
}

// eventuallyHasExpectedAPIKey checks if the API key is present in the orchestrator resources payloads.
func (suite *k8sSuite) eventuallyHasExpectedAPIKey(apiKey string) {
	hasKey := func() bool {
		keys, err := suite.Env().FakeIntake.Client().GetOrchestratorResourcesPayloadAPIKeys()
		if err != nil {
			return false
		}

		for _, key := range keys {
			if key == apiKey {
				return true
			}
		}
		return false
	}

	assert.Eventually(suite.T(), hasKey, 10*time.Minute, 10*time.Second)
}

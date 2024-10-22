// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils contains util functions for OTel e2e tests
package utils

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	extension "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def"
)

// TestOTelAgentInstalled checks that the OTel Agent is installed in the test suite
func TestOTelAgentInstalled(s OTelTestSuite) {
	agent := getAgentPod(s)
	assert.Contains(s.T(), agent.ObjectMeta.String(), "otel-agent")
}

// TestOTelFlare tests that the OTel Agent flare functionality works as expected
func TestOTelFlare(s OTelTestSuite, providedCfg string, fullCfg string, sources string) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	s.T().Log("Starting flare")
	agent := getAgentPod(s)
	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", agent.Name, "agent", []string{"agent", "flare", "--email", "e2e@test.com", "--send"})
	require.NoError(s.T(), err, "Failed to execute flare")
	require.Empty(s.T(), stderr)
	require.NotNil(s.T(), stdout)

	s.T().Log("Getting latest flare")
	flare, err := s.Env().FakeIntake.Client().GetLatestFlare()
	require.NoError(s.T(), err, "Failed to get latest flare")
	otelFolder, otelFlareFolder := false, false
	var otelResponse string
	for _, filename := range flare.GetFilenames() {
		if strings.Contains(filename, "/otel/") {
			otelFolder = true
		}
		if strings.Contains(filename, "/otel/otel-flare/") {
			otelFlareFolder = true
		}
		if strings.Contains(filename, "otel/otel-response.json") {
			otelResponse = filename
		}
	}
	assert.True(s.T(), otelFolder)
	assert.True(s.T(), otelFlareFolder)
	otelResponseContent, err := flare.GetFileContent(otelResponse)
	s.T().Log("Got flare otel-response.json", otelResponseContent)
	require.NoError(s.T(), err)
	var resp extension.Response
	require.NoError(s.T(), json.Unmarshal([]byte(otelResponseContent), &resp))

	assert.Equal(s.T(), "otel-agent", resp.AgentCommand)
	assert.Equal(s.T(), "Datadog Agent OpenTelemetry Collector", resp.AgentDesc)
	assert.Equal(s.T(), "", resp.RuntimeOverrideConfig)

	validateConfigs(s.T(), providedCfg, resp.CustomerConfig)
	validateConfigs(s.T(), fullCfg, resp.RuntimeConfig)

	srcJSONStr, err := json.Marshal(resp.Sources)
	require.NoError(s.T(), err)
	assert.JSONEq(s.T(), sources, string(srcJSONStr))
}

func getAgentPod(s OTelTestSuite) corev1.Pod {
	res, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
	})
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), res.Items)
	return res.Items[0]
}

func validateConfigs(t *testing.T, expectedCfg string, actualCfg string) {
	var actualConfRaw map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(actualCfg), &actualConfRaw))

	// traces, metrics and logs endpoints are set to the fake intake address in the config
	// these endpoints should be ignored in the comparison
	exps, _ := actualConfRaw["exporters"].(map[string]any)
	ddExp, _ := exps["datadog"].(map[string]any)
	tcfg := ddExp["traces"].(map[string]any)
	delete(tcfg, "endpoint")
	mcfg := ddExp["metrics"].(map[string]any)
	delete(mcfg, "endpoint")
	lcfg := ddExp["logs"].(map[string]any)
	delete(lcfg, "endpoint")

	var expectedConfRaw map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(expectedCfg), &expectedConfRaw))

	assert.Equal(t, expectedConfRaw, actualConfRaw)
}

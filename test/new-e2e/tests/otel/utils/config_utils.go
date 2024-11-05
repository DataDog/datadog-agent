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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	extension "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/fakeintake/client/flare"
)

// TestOTelAgentInstalled checks that the OTel Agent is installed in the test suite
func TestOTelAgentInstalled(s OTelTestSuite) {
	agent := getAgentPod(s)
	assert.Contains(s.T(), agent.ObjectMeta.String(), "otel-agent")
}

var otelFlareFilesCommon = []string{
	"otel/otel-response.json",
	"otel/otel-flare/customer.cfg",
	"otel/otel-flare/env.cfg",
	"otel/otel-flare/environment.json",
	"otel/otel-flare/runtime.cfg",
	"otel/otel-flare/runtime_override.cfg",
	"otel/otel-flare/health_check/dd-autoconfigured.dat",
	"otel/otel-flare/pprof/dd-autoconfigured_debug_pprof_heap.dat",
	"otel/otel-flare/pprof/dd-autoconfigured_debug_pprof_allocs.dat",
	// "otel/otel-flare/pprof/dd-autoconfigured_debug_pprof_profile.dat",
	"otel/otel-flare/command.txt",
	"otel/otel-flare/ext.txt",
}

var otelFlareFilesZpages = []string{
	"otel/otel-flare/zpages/dd-autoconfigured_debug_tracez.dat",
	"otel/otel-flare/zpages/dd-autoconfigured_debug_pipelinez.dat",
	"otel/otel-flare/zpages/dd-autoconfigured_debug_extensionz.dat",
	"otel/otel-flare/zpages/dd-autoconfigured_debug_featurez.dat",
	"otel/otel-flare/zpages/dd-autoconfigured_debug_servicez.dat",
}

// TestOTelFlareExtensionResponse tests that the OTel Agent DD flare extension returns expected responses
func TestOTelFlareExtensionResponse(s OTelTestSuite, providedCfg string, fullCfg string, sources string) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)
	agent := getAgentPod(s)

	s.T().Log("Starting flare")
	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", agent.Name, "agent", []string{"agent", "flare", "--email", "e2e@test.com", "--send"})
	require.NoError(s.T(), err, "Failed to execute flare")
	require.Empty(s.T(), stderr)
	require.NotNil(s.T(), stdout)

	flare, err := s.Env().FakeIntake.Client().GetLatestFlare()
	require.NoError(s.T(), err)
	otelflares := fetchFromFlare(s.T(), flare)

	require.Contains(s.T(), otelflares, "otel/otel-response.json")
	var resp extension.Response
	require.NoError(s.T(), json.Unmarshal([]byte(otelflares["otel/otel-response.json"]), &resp))

	assert.Equal(s.T(), "otel-agent", resp.AgentCommand)
	assert.Equal(s.T(), "Datadog Agent OpenTelemetry Collector", resp.AgentDesc)
	assert.Equal(s.T(), "", resp.RuntimeOverrideConfig)

	validateConfigs(s.T(), providedCfg, resp.CustomerConfig)
	validateConfigs(s.T(), fullCfg, resp.RuntimeConfig)

	srcJSONStr, err := json.Marshal(resp.Sources)
	require.NoError(s.T(), err)
	assert.JSONEq(s.T(), sources, string(srcJSONStr))
}

// TestOTelFlareFiles tests that the OTel Agent flares contain the expected files
func TestOTelFlareFiles(s OTelTestSuite) {
	flake.Mark(s.T())
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)
	agent := getAgentPod(s)

	s.T().Log("Starting flare")
	hasZpages := false
	otelflares := make(map[string]string)
	timeout := time.Now().Add(10 * time.Minute)
	for i := 1; time.Now().Before(timeout); i++ {
		stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", agent.Name, "agent", []string{"agent", "flare", "--email", "e2e@test.com", "--send"})
		require.NoError(s.T(), err, "Failed to execute flare")
		require.Empty(s.T(), stderr)
		require.NotNil(s.T(), stdout)

		s.T().Logf("Getting latest flare, attempt %d", i)
		flare, err := s.Env().FakeIntake.Client().GetLatestFlare()
		require.NoError(s.T(), err)
		otelflaresResp := fetchFromFlare(s.T(), flare)
		for k, v := range otelflaresResp {
			s.T().Log("Got otel flare: ", k)
			otelflares[k] = v
		}

		if len(otelflares) >= len(otelFlareFilesCommon)+len(otelFlareFilesZpages) {
			hasZpages = true
			break
		}

		time.Sleep(30 * time.Second)
	}

	otelFlareFiles := otelFlareFilesCommon
	if hasZpages {
		otelFlareFiles = append(otelFlareFiles, otelFlareFilesZpages...)
	}
	for _, otelFlareFile := range otelFlareFiles {
		assert.Contains(s.T(), otelflares, otelFlareFile, "missing ", otelFlareFile)
	}

	assert.Contains(s.T(), otelflares["otel/otel-flare/health_check/dd-autoconfigured.dat"], `"status":"Server available"`)
}

func getAgentPod(s OTelTestSuite) corev1.Pod {
	res, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
	})
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), res.Items)
	return res.Items[0]
}

func fetchFromFlare(t *testing.T, flare flare.Flare) map[string]string {
	otelflares := make(map[string]string)
	for _, filename := range flare.GetFilenames() {
		if !strings.Contains(filename, "/otel/") {
			continue
		}

		if strings.HasSuffix(filename, ".json") || strings.HasSuffix(filename, ".dat") || strings.HasSuffix(filename, ".txt") || strings.HasSuffix(filename, ".cfg") {
			cnt, err := flare.GetFileContent(filename)
			require.NoError(t, err)
			parts := strings.SplitN(filename, "/", 2)
			require.Len(t, parts, 2)
			otelflares[parts[1]] = cnt
		}
	}
	return otelflares
}

func validateConfigs(t *testing.T, expectedCfg string, actualCfg string) {
	var actualConfRaw map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(actualCfg), &actualConfRaw))

	// Traces, metrics and logs endpoints are set dynamically to the fake intake address in the config
	// These endpoints vary from test to test and should be ignored in the comparison
	exps, _ := actualConfRaw["exporters"].(map[string]any)
	ddExp, _ := exps["datadog"].(map[string]any)
	tcfg := ddExp["traces"].(map[string]any)
	delete(tcfg, "endpoint")
	mcfg := ddExp["metrics"].(map[string]any)
	delete(mcfg, "endpoint")
	lcfg := ddExp["logs"].(map[string]any)
	delete(lcfg, "endpoint")

	actualCfgBytes, err := yaml.Marshal(actualConfRaw)
	require.NoError(t, err)
	actualCfg = string(actualCfgBytes)

	assert.YAMLEq(t, expectedCfg, actualCfg)
}

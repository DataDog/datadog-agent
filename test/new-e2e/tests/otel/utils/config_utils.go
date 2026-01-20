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

	extensiontypes "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/types"
	"github.com/DataDog/datadog-agent/test/fakeintake/client/flare"
)

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname              string                 `json:"hostname"`
	Timestamp             int64                  `json:"timestamp"`
	OTelCollectorMetadata *OTelCollectorMetadata `json:"otel_collector"`
	UUID                  string                 `json:"uuid"`
}

// OTelCollectorMetadata represents the datadog extension metadata payload
type OTelCollectorMetadata struct {
	BuildInfo         BuildInfo `json:"build_info"`
	FullConfiguration string    `json:"full_configuration"`
}

type BuildInfo struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

// TestOTelAgentInstalled checks that the OTel Agent is installed in the test suite
func TestOTelAgentInstalled(s OTelTestSuite) {
	agent := getAgentPod(s)
	assert.Contains(s.T(), agent.ObjectMeta.String(), "otel-agent")
}

// TestOTelGatewayInstalled checks that the OTel Gateway collector is installed in the test suite
func TestOTelGatewayInstalled(s OTelTestSuite) {
	agent := getGatewayPod(s)
	assert.Contains(s.T(), agent.ObjectMeta.String(), "otel-agent-gateway")

	services := getAgentServices(s)
	gatewayService := false
	for _, service := range services {
		if service.Name == "dda-linux-datadog-otel-agent-gateway" {
			gatewayService = true
		}
	}
	assert.True(s.T(), gatewayService)
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
	var resp extensiontypes.Response
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

// TestDatadogExtensionPayload tests that the OTel Agent DD extension returns expected responses
func TestDatadogExtensionPayload(s OTelTestSuite, fullCfg string) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)
	agent := getAgentPod(s)

	s.T().Log("Starting diagnose")
	stdout, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", agent.Name, "agent", []string{"curl", "http://localhost:9875/metadata"})
	require.NoError(s.T(), err, "Failed to execute diagnose")
	require.NotNil(s.T(), stdout)
	ind := strings.Index(stdout, "{")
	require.NotEqual(s.T(), ind, -1)
	rawPayload := stdout[ind:]

	var payload Payload
	err = json.Unmarshal([]byte(rawPayload), &payload)
	if err != nil {
		s.T().Fatal(err)
	}
	s.T().Log("Got metadata payload")

	assert.Equal(s.T(), "otel-agent", payload.OTelCollectorMetadata.BuildInfo.Command)
	assert.Equal(s.T(), "Datadog Agent OpenTelemetry Collector", payload.OTelCollectorMetadata.BuildInfo.Description)
	validateConfigs(s.T(), fullCfg, payload.OTelCollectorMetadata.FullConfiguration)
}

func getAgentPod(s OTelTestSuite) corev1.Pod {
	res, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
	})
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), res.Items)
	return res.Items[0]
}

func getGatewayPod(s OTelTestSuite) corev1.Pod {
	res, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", "dda-linux-datadog-otel-agent-gateway").String(),
	})
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), res.Items)
	return res.Items[0]
}

func getAgentServices(s OTelTestSuite) []corev1.Service {
	res, err := s.Env().KubernetesCluster.Client().CoreV1().Services("datadog").List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app.kubernetes.io/name", "dda-linux-datadog").String(),
	})
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), res.Items)
	return res.Items
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
	if ddExp, ok := exps["datadog"]; ok {
		ddExpCfg := ddExp.(map[string]any)
		tcfg := ddExpCfg["traces"].(map[string]any)
		delete(tcfg, "endpoint")
		mcfg := ddExpCfg["metrics"].(map[string]any)
		delete(mcfg, "endpoint")
		lcfg := ddExpCfg["logs"].(map[string]any)
		delete(lcfg, "endpoint")
	}

	actualCfgBytes, err := yaml.Marshal(actualConfRaw)
	require.NoError(t, err)
	actualCfg = string(actualCfgBytes)

	assert.YAMLEq(t, expectedCfg, actualCfg)
}

// TestCoreAgentStatusCmd tests the core agent status command contains the OTel Agent status as expected
func TestCoreAgentStatusCmd(s OTelTestSuite) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)
	agent := getAgentPod(s)

	s.T().Log("Calling status command in core agent")
	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", agent.Name, "agent", []string{"agent", "status", "otel agent"})
	require.NoError(s.T(), err, "Failed to execute config")
	require.Empty(s.T(), stderr)
	validateStatus(s.T(), stdout)
}

// TestOTelAgentStatusCmd tests the OTel Agent status subcommand returns as expected
func TestOTelAgentStatusCmd(s OTelTestSuite) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)
	agent := getAgentPod(s)

	s.T().Log("Calling status command in otel agent")
	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", agent.Name, "otel-agent", []string{"otel-agent", "status"})
	require.NoError(s.T(), err, "Failed to execute config")
	require.Empty(s.T(), stderr)
	validateStatus(s.T(), stdout)
}

func validateStatus(t *testing.T, status string) {
	require.NotNil(t, status)
	require.Contains(t, status, "OTel Agent")
	require.Contains(t, status, "Status: Running")
	require.Contains(t, status, "Agent Version:")
	require.Contains(t, status, "Collector Version:")
	require.Contains(t, status, "Spans Accepted:")
	require.Contains(t, status, "Metric Points Accepted:")
	require.Contains(t, status, "Log Records Accepted:")
	require.Contains(t, status, "Spans Sent:")
	require.Contains(t, status, "Metric Points Sent:")
	require.Contains(t, status, "Log Records Sent:")
}

// TestOTelAgentFlareCmd tests the OTel Agent flare subcommand executes successfully and validates the flare contents
func TestOTelAgentFlareCmd(s OTelTestSuite) {
	testOTelFlareCmd(s, getAgentPod(s), "otel agent")
}

// TestOTelGatewayFlareCmd tests the OTel Gateway flare subcommand executes successfully and validates the flare contents
func TestOTelGatewayFlareCmd(s OTelTestSuite) {
	testOTelFlareCmd(s, getGatewayPod(s), "otel gateway")
}

// testOTelFlareCmd is the common implementation for testing the flare command on any OTel pod
func testOTelFlareCmd(s OTelTestSuite, pod corev1.Pod, podType string) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)

	s.T().Logf("Calling flare command in %s", podType)
	// Note: We're not uploading the flare, just creating it locally
	// The command will create a zip file and decline upload with "n"
	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", pod.Name, "otel-agent", []string{"sh", "-c", "echo n | otel-agent flare --email test@example.com 2>&1"})

	s.T().Logf("Flare command stdout: %s", stdout)
	s.T().Logf("Flare command stderr: %s", stderr)

	// The command should succeed (or gracefully handle the declined upload)
	// We expect either success or the "Aborting" message when declining the upload
	if err != nil {
		// If there's an error, it should be a graceful abort, not a crash
		require.Contains(s.T(), stdout, "Aborting", "Expected graceful abort message in output")
	}

	// Validate the flare was created and extract the file path
	validateOTelFlareOutput(s.T(), stdout)

	// Extract flare file path from output
	flarePath := extractFlarePathFromOutput(stdout)
	if flarePath == "" {
		s.T().Log("Could not extract flare path from output, skipping content validation")
		return
	}

	s.T().Logf("Flare file created at: %s", flarePath)

	// Read and validate the flare file contents
	validateOTelFlareContents(s, pod, flarePath, podType)
}

func validateOTelFlareOutput(t *testing.T, output string) {
	require.NotEmpty(t, output)

	// Check that the flare collection process started
	require.Contains(t, output, "Collecting", "Expected flare collection message")

	// Check for diagnostic data collection messages
	// The output should contain messages about collecting configuration data
	anyValidMessage :=
		strings.Contains(output, "Collecting diagnostic data") ||
			strings.Contains(output, "Collecting OTel configuration data") ||
			strings.Contains(output, "Flare archive created") ||
			strings.Contains(output, "otel-agent-flare_")
	require.True(t, anyValidMessage, "Expected to find flare collection messages in output")
}

// extractFlarePathFromOutput extracts the flare file path from the command output
func extractFlarePathFromOutput(output string) string {
	// Look for lines like "Flare archive created: /tmp/otel-agent-flare_2025-12-22_15-04-05.zip"
	// or "/tmp/otel-agent-flare_2025-12-22_15-04-05.zip is going to be uploaded to Datadog"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "otel-agent-flare_") && strings.Contains(line, ".zip") {
			// Extract the path
			start := strings.Index(line, "/tmp/")
			if start == -1 {
				start = strings.Index(line, "/var/")
			}
			if start != -1 {
				end := strings.Index(line[start:], ".zip")
				if end != -1 {
					return line[start : start+end+4] // +4 for ".zip"
				}
			}
		}
	}
	return ""
}

// validateOTelFlareContents validates the contents of the otel flare archive
func validateOTelFlareContents(s OTelTestSuite, pod corev1.Pod, flarePath string, podType string) {
	// List contents of the zip file
	s.T().Log("Listing flare archive contents")
	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", pod.Name, "otel-agent", []string{"unzip", "-l", flarePath})
	if err != nil {
		s.T().Logf("Could not list flare contents (unzip may not be available): %v, stderr: %s", err, stderr)
		// If unzip is not available, we can still validate that the file exists
		stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", pod.Name, "otel-agent", []string{"ls", "-lh", flarePath})
		require.NoError(s.T(), err, "Failed to check flare file exists")
		require.Empty(s.T(), stderr)
		require.Contains(s.T(), stdout, "otel-agent-flare_", "Flare file should exist")
		s.T().Log("Flare file exists but cannot extract contents without unzip utility")
		return
	}

	s.T().Logf("Flare archive contents:\n%s", stdout)

	// Validate expected files are in the archive
	// These files should be created by the otel-agent flare command
	expectedFiles := []string{
		"otel-response.json",
		"build-info.txt",
		"config/customer-config.yaml",
		"config/env-config.yaml",
		"config/runtime-config.yaml",
		"environment.json",
		"otel/otel-flare/health_check/dd-autoconfigured.dat",
		"otel/otel-flare/pprof/dd-autoconfigured_debug_pprof_heap.dat",
		"otel/otel-flare/pprof/dd-autoconfigured_debug_pprof_allocs.dat",
	}

	for _, expectedFile := range expectedFiles {
		assert.Contains(s.T(), stdout, expectedFile, "Flare should contain "+expectedFile)
	}

	// Try to extract and validate the otel-response.json content
	s.T().Log("Extracting and validating otel-response.json")
	extractCmd := []string{"sh", "-c", "unzip -p " + flarePath + " otel-response.json"}
	responseJSON, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", pod.Name, "otel-agent", extractCmd)
	if err != nil {
		s.T().Logf("Could not extract otel-response.json: %v, stderr: %s", err, stderr)
		return
	}

	// Parse and validate the response
	var resp extensiontypes.Response
	err = json.Unmarshal([]byte(responseJSON), &resp)
	require.NoError(s.T(), err, "Should be able to parse otel-response.json")

	// Validate response structure (similar to TestOTelFlareExtensionResponse)
	assert.Equal(s.T(), "otel-agent", resp.AgentCommand)
	assert.Equal(s.T(), "Datadog OTel Agent", resp.AgentDesc)
	assert.Equal(s.T(), "", resp.RuntimeOverrideConfig)
	assert.NotEmpty(s.T(), resp.AgentVersion, "Agent version should not be empty")

	// Validate that configs are not empty
	assert.NotEmpty(s.T(), resp.CustomerConfig, "Customer config should not be empty")
	assert.NotEmpty(s.T(), resp.RuntimeConfig, "Runtime config should not be empty")
	assert.NotEmpty(s.T(), resp.Environment, "Environment should not be empty")

	s.T().Logf("OTel %s flare validation completed successfully", podType)
}

// TestCoreAgentConfigCmd tests the output of core agent's config command contains the embedded collector's config
func TestCoreAgentConfigCmd(s OTelTestSuite, expectedCfg string) {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	require.NoError(s.T(), err)
	agent := getAgentPod(s)

	s.T().Log("Calling 'config otel-agent' command in core agent")
	stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", agent.Name, "agent", []string{"agent", "config", "otel-agent"})
	require.NoError(s.T(), err, "Failed to execute config")
	require.Empty(s.T(), stderr)
	require.NotNil(s.T(), stdout)
	s.T().Log("Full output of 'config otel-agent' command in core agent\n", stdout)
	assert.Contains(s.T(), stdout, expectedCfg)
}

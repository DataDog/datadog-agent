// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package helmagent provides functions to install and configure the Datadog
// Agent on a Kubernetes cluster via Helm, without relying on Pulumi.
package helmagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultNamespace   = "datadog"
	defaultReleaseName = "dda"
	defaultChartRepo   = "https://helm.datadoghq.com"
	defaultChartName   = "datadog"
	// chartVersion must match the version pinned by the Pulumi installer
	// (HelmVersion in components/datadog/agent/kubernetes_helm.go).
	chartVersion = "3.155.1"
)

// Install installs the Datadog Agent on a Kubernetes cluster via Helm,
// configures it, and waits for the agent pods to be ready.
// It populates env.Agent with the initialized agent component.
//
// Usage in SetupSuite:
//
//	helmagent.Install(s.T(), s.Env(),
//	    kubernetesagentparams.WithHelmValues(myValues),
//	    kubernetesagentparams.WithNamespace("datadog"),
//	)
func Install(t *testing.T, env *environments.Kubernetes, opts ...kubernetesagentparams.Option) {
	t.Helper()
	require.NotNil(t, env.KubernetesCluster, "helmagent.Install: KubernetesCluster is nil, infrastructure must be provisioned first")

	p, err := buildParams(opts)
	require.NoError(t, err, "failed to build helm agent params")

	// Write kubeconfig to temp file for helm CLI
	kubeconfigFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	require.NoError(t, err)
	defer os.Remove(kubeconfigFile.Name())
	_, err = kubeconfigFile.WriteString(env.KubernetesCluster.KubeConfig)
	require.NoError(t, err)
	require.NoError(t, kubeconfigFile.Close())

	k8sClient := env.KubernetesCluster.Client()
	ctx := context.Background()

	// Create namespace
	_, err = k8sClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: p.Namespace},
	}, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		require.NoError(t, err, "failed to create namespace %s", p.Namespace)
	}

	// Create credentials secret
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	require.NoError(t, err, "failed to get API key")
	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	require.NoError(t, err, "failed to get APP key")

	secretName := defaultReleaseName + "-datadog-credentials"
	_, err = k8sClient.CoreV1().Secrets(p.Namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: p.Namespace},
		StringData: map[string]string{
			"api-key": strings.TrimSpace(apiKey),
			"app-key": strings.TrimSpace(appKey),
		},
	}, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		require.NoError(t, err, "failed to create credentials secret")
	}

	// Build values YAML
	valuesYAML := buildValuesYAML(t, env, p, secretName)

	// Write values to temp file
	valuesFile, err := os.CreateTemp("", "helm-values-*.yaml")
	require.NoError(t, err)
	defer os.Remove(valuesFile.Name())
	_, err = valuesFile.WriteString(valuesYAML)
	require.NoError(t, err)
	require.NoError(t, valuesFile.Close())

	// Run helm upgrade --install. The chart version is pinned to match
	// what the Pulumi path installs, ensuring identical agent behavior.
	helmArgs := []string{
		"upgrade", "--install", defaultReleaseName, p.HelmChartPath,
		"--namespace", p.Namespace,
		"--values", valuesFile.Name(),
		"--kubeconfig", kubeconfigFile.Name(),
		"--wait",
		"--timeout", fmt.Sprintf("%ds", p.TimeoutSeconds),
		"--version", chartVersion,
		"--dependency-update",
	}
	if p.HelmRepoURL != "" {
		helmArgs = append(helmArgs, "--repo", p.HelmRepoURL)
	}

	t.Logf("Running: helm %s", strings.Join(helmArgs, " "))
	cmd := exec.Command("helm", helmArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("helm output:\n%s", string(output))
	}
	require.NoError(t, err, "helm install failed")
	t.Logf("helm output:\n%s", string(output))

	// Initialize agent component with known label selectors from the Helm chart
	env.Agent = &components.KubernetesAgent{}
	env.Agent.LinuxNodeAgent.LabelSelectors = map[string]string{
		"app": defaultReleaseName + "-datadog",
	}
	env.Agent.LinuxClusterAgent.LabelSelectors = map[string]string{
		"app": defaultReleaseName + "-datadog-cluster-agent",
	}
	env.Agent.LinuxClusterChecks.LabelSelectors = map[string]string{
		"app": defaultReleaseName + "-datadog-clusterchecks",
	}

	// Store baseline options and set the Helm installer for Configure
	env.Agent.SetBaseOptions(opts...)
	env.Agent.Installer = &helmInstaller{env: env}

	// Wait for agent pods to be ready
	waitForAgentReady(t, env, p.Namespace)
}

// helmInstaller implements components.KubernetesAgentInstaller for Helm-based agents.
type helmInstaller struct {
	env *environments.Kubernetes
}

// Upgrade runs helm upgrade with the given options.
func (h *helmInstaller) Upgrade(t *testing.T, opts []kubernetesagentparams.Option) error {
	Install(t, h.env, opts...)
	return nil
}

// buildParams creates Params from options with defaults.
func buildParams(opts []kubernetesagentparams.Option) (*kubernetesagentparams.Params, error) {
	p := &kubernetesagentparams.Params{
		Namespace:      defaultNamespace,
		HelmRepoURL:    defaultChartRepo,
		HelmChartPath:  defaultChartName,
		TimeoutSeconds: 600,
	}
	for _, opt := range opts {
		if err := opt(p); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// buildValuesYAML generates the Helm values YAML that configures the agent.
// This replicates the values produced by the Pulumi buildLinuxHelmValues
// function in kubernetes_helm.go to ensure identical agent behavior.
func buildValuesYAML(t *testing.T, env *environments.Kubernetes, p *kubernetesagentparams.Params, secretName string) string {
	t.Helper()

	// Cluster name is required for KinD/non-cloud clusters where the agent
	// cannot detect it via cloud provider metadata. Mirrors WithClusterName
	// in kindvm/run.go.
	clusterName := env.KubernetesCluster.ClusterName

	// Base values — mirrors buildLinuxHelmValues in kubernetes_helm.go
	base := fmt.Sprintf(`datadog:
  apiKeyExistingSecret: %[1]s
  appKeyExistingSecret: %[1]s
  clusterName: %[2]s
  leaderElectionResource: ""
  checksCardinality: high
  namespaceAnnotationsAsTags:
    related_email: email
  kubernetesResourcesAnnotationsAsTags:
    deployments.apps:
      x-sub-team: sub-team
    pods:
      x-parent-name: parent-name
    namespaces:
      related_email: mail
  kubernetesResourcesLabelsAsTags:
    deployments.apps:
      x-team: team
    pods:
      x-parent-type: domain
    namespaces:
      related_org: org
    nodes:
      kubernetes.io/os: os
      kubernetes.io/arch: arch
      eks.amazonaws.com/nodegroup-image: nodegroup-image
  originDetectionUnified:
    enabled: true
  logs:
    enabled: true
    containerCollectAll: true
  dogstatsd:
    originDetection: true
    tagCardinality: high
    useHostPort: true
  apm:
    portEnabled: true
    instrumentation:
      enabled: true
      enabledNamespaces:
        - workload-mutated-lib-injection
      language_detection:
        enabled: true
  processAgent:
    processCollection: true
  helmCheck:
    enabled: true
  kubeStateMetricsCore:
    enabled: true
    collectVpaMetrics: true
    useClusterCheckRunners: true
    tags:
      - kube_instance_tag:static
  prometheusScrape:
    enabled: true
  sbom:
    host:
      enabled: true
    containerImage:
      enabled: true
      uncompressedLayersSupport: true
  env:
    - name: DD_EC2_METADATA_TIMEOUT
      value: "5000"
    - name: DD_TELEMETRY_ENABLED
      value: "true"
    - name: DD_TELEMETRY_CHECKS
      value: "*"
  autoscaling:
    workload:
      enabled: true
agents:
  priorityClassCreate: true
  containers:
    agent:
      env:
        - name: DD_LANGUAGE_DETECTION_REPORTING_REFRESH_PERIOD
          value: "1m"
      resources:
        requests:
          cpu: 400m
          memory: 500Mi
        limits:
          cpu: 1000m
          memory: 700Mi
    processAgent:
      resources:
        requests:
          cpu: 50m
          memory: 150Mi
        limits:
          cpu: 200m
          memory: 200Mi
    traceAgent:
      resources:
        requests:
          cpu: 10m
          memory: 120Mi
        limits:
          cpu: 200m
          memory: 200Mi
clusterAgent:
  enabled: true
  replicas: 2
  metricsProvider:
    enabled: true
    useDatadogMetrics: true
  admissionController:
    agentSidecarInjection:
      enabled: true
      provider: fargate
  resources:
    requests:
      cpu: 50m
      memory: 150Mi
    limits:
      cpu: 200m
      memory: 200Mi
  env:
    - name: DD_CLUSTER_CHECKS_CRD_COLLECTION
      value: "true"
    - name: DD_EC2_METADATA_TIMEOUT
      value: "5000"
    - name: DD_ADMISSION_CONTROLLER_AUTO_INSTRUMENTATION_INJECT_AUTO_DETECTED_LIBRARIES
      value: "true"
    - name: DD_ADMISSION_CONTROLLER_AGENT_SIDECAR_KUBELET_API_LOGGING_ENABLED
      value: "true"
    - name: DD_ADMISSION_CONTROLLER_AUTO_INSTRUMENTATION_GRADUAL_ROLLOUT_ENABLED
      value: "false"
clusterChecksRunner:
  enabled: true
  env:
    - name: DD_CLC_RUNNER_REMOTE_TAGGER_ENABLED
      value: "true"
  resources:
    requests:
      cpu: 20m
      memory: 300Mi
    limits:
      cpu: 200m
      memory: 400Mi
`, secretName, clusterName)

	// Agent and cluster-agent image configuration
	imageValues := buildImageValues(t, p)
	if imageValues != "" {
		base = mustMerge(t, base, imageValues)
	}

	// Fakeintake configuration (env vars on agent/clusterAgent/clusterChecksRunner pods)
	if env.FakeIntake != nil && env.FakeIntake.URL != "" {
		base = mustMerge(t, base, buildFakeintakeValues(env.FakeIntake, p.DualShipping))
	}

	// Merge user-provided values last (highest priority)
	for _, raw := range p.HelmValuesRaw {
		base = mustMerge(t, base, raw)
	}

	return base
}

func mustMerge(t *testing.T, base, overlay string) string {
	t.Helper()
	merged, err := utils.MergeYAMLWithSlices(base, overlay)
	require.NoError(t, err, "failed to merge helm values")
	return merged
}

// buildImageValues generates Helm values for agent/cluster-agent images based
// on the runner profile (pipeline ID, commit SHA) or user-provided image paths.
func buildImageValues(t *testing.T, p *kubernetesagentparams.Params) string {
	t.Helper()

	// User-provided full image paths take precedence
	if p.AgentFullImagePath != "" {
		repo, tag := parseImageRef(p.AgentFullImagePath)
		values := fmt.Sprintf("agents:\n  image:\n    repository: %s\n    tag: \"%s\"\n    doNotCheckTag: true\n", repo, tag)
		if p.ClusterAgentFullImagePath != "" {
			cRepo, cTag := parseImageRef(p.ClusterAgentFullImagePath)
			values += fmt.Sprintf("clusterAgent:\n  image:\n    repository: %s\n    tag: \"%s\"\n", cRepo, cTag)
		}
		return values
	}

	// Check for pipeline-based image from runner profile
	profile := runner.GetProfile()
	pipelineID, _ := profile.ParamStore().GetWithDefault(parameters.PipelineID, "")
	commitSHA, _ := profile.ParamStore().GetWithDefault(parameters.CommitSHA, "")

	if pipelineID != "" && commitSHA != "" {
		tag := fmt.Sprintf("%s-%s", pipelineID, commitSHA)
		return fmt.Sprintf(`agents:
  image:
    repository: gcr.io/datadoghq/agent
    tag: "%s"
    doNotCheckTag: true
clusterAgent:
  image:
    repository: gcr.io/datadoghq/cluster-agent
    tag: "%s"
`, tag, tag)
	}

	// No specific image — use chart defaults (latest stable)
	return ""
}

// parseImageRef splits "repo/image:tag" into ("repo/image", "tag").
func parseImageRef(ref string) (string, string) {
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		return ref[:idx], ref[idx+1:]
	}
	return ref, "latest"
}

// buildFakeintakeValues generates Helm values that point the agent to fakeintake.
// Mirrors configureFakeintake in kubernetes_helm.go.
// When dualShipping is true, uses additional-endpoints so traffic goes to both
// Datadog and fakeintake (matching the dual-shipping Pulumi path).
func buildFakeintakeValues(fi *components.FakeIntake, dualShipping bool) string {
	var envVars string
	if dualShipping {
		useSSL := strings.HasPrefix(fi.URL, "https://")
		host := fi.Host
		port := fi.Port
		envVars = fmt.Sprintf(`    - name: DD_SKIP_SSL_VALIDATION
      value: "true"
    - name: DD_REMOTE_CONFIGURATION_NO_TLS_VALIDATION
      value: "true"
    - name: DD_ADDITIONAL_ENDPOINTS
      value: '{"%[1]s": ["FAKEAPIKEY"]}'
    - name: DD_PROCESS_ADDITIONAL_ENDPOINTS
      value: '{"%[1]s": ["FAKEAPIKEY"]}'
    - name: DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_ADDITIONAL_ENDPOINTS
      value: '{"%[1]s": ["FAKEAPIKEY"]}'
    - name: DD_LOGS_CONFIG_ADDITIONAL_ENDPOINTS
      value: '[{"host": "%[2]s", "port": %[3]d, "use_ssl": %[4]t}]'
    - name: DD_LOGS_CONFIG_USE_HTTP
      value: "true"
    - name: DD_CONTAINER_IMAGE_ADDITIONAL_ENDPOINTS
      value: '[{"host": "%[2]s", "use_ssl": %[4]t}]'
    - name: DD_CONTAINER_LIFECYCLE_ADDITIONAL_ENDPOINTS
      value: '[{"host": "%[2]s", "use_ssl": %[4]t}]'
    - name: DD_SBOM_ADDITIONAL_ENDPOINTS
      value: '[{"host": "%[2]s", "use_ssl": %[4]t}]'`, fi.URL, host, port, useSSL)
	} else {
		envVars = fmt.Sprintf(`    - name: DD_DD_URL
      value: "%[1]s"
    - name: DD_PROCESS_CONFIG_PROCESS_DD_URL
      value: "%[1]s"
    - name: DD_APM_DD_URL
      value: "%[1]s"
    - name: DD_LOGS_CONFIG_LOGS_DD_URL
      value: "%[1]s"
    - name: DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_DD_URL
      value: "%[1]s"
    - name: DD_SKIP_SSL_VALIDATION
      value: "true"
    - name: DD_REMOTE_CONFIGURATION_NO_TLS_VALIDATION
      value: "true"
    - name: DD_LOGS_CONFIG_USE_HTTP
      value: "true"`, fi.URL)
	}

	return fmt.Sprintf(`datadog:
  env:
%[1]s
clusterAgent:
  env:
%[1]s
clusterChecksRunner:
  env:
%[1]s
`, envVars)
}

// waitForAgentReady waits for at least one agent pod to be ready.
func waitForAgentReady(t *testing.T, env *environments.Kubernetes, namespace string) {
	t.Helper()
	k8sClient := env.KubernetesCluster.Client()
	ctx := context.Background()

	require.Eventually(t, func() bool {
		// Try the standard label selectors used by the Datadog Helm chart
		for _, selector := range []string{
			"app.kubernetes.io/component=agent",
			"app=datadog-agent",
		} {
			pods, err := k8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: selector,
			})
			if err != nil || len(pods.Items) == 0 {
				continue
			}
			for _, pod := range pods.Items {
				for _, cond := range pod.Status.Conditions {
					if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 5*time.Minute, 10*time.Second, "agent pods not ready in namespace %s", namespace)
}

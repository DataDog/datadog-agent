// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package helmagent provides functions to install and configure the Datadog
// Agent on a Kubernetes cluster via Helm, without relying on Pulumi.
package helmagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
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

	// Create image pull secret when registry credentials are configured.
	// Mirrors utils.NewImagePullSecret in common/utils/kubernetes.go.
	pullSecretName, err := createImagePullSecret(ctx, k8sClient, p.Namespace)
	require.NoError(t, err, "failed to create image pull secret")

	// Build and merge all values
	valuesYAML := buildValuesYAML(t, env, p, secretName, pullSecretName)

	// Parse values into map for the Helm SDK
	vals := map[string]interface{}{}
	require.NoError(t, yaml.Unmarshal([]byte(valuesYAML), &vals), "failed to parse helm values")

	// Run helm upgrade --install via the Go SDK (no helm CLI required)
	require.NoError(t,
		helmUpgradeInstall(t, env.KubernetesCluster.KubeConfig, p, vals),
		"helm upgrade --install failed",
	)

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
}

// helmUpgradeInstall runs helm install or helm upgrade using the Helm Go SDK.
// No helm CLI required; kubeconfig is used in-memory.
// action.Upgrade.Install = true is purely informational in Helm v3 and does not
// handle the install case — we detect release existence and branch manually.
func helmUpgradeInstall(t *testing.T, kubeconfig string, p *kubernetesagentparams.Params, vals map[string]interface{}) error {
	t.Helper()

	// Build RESTClientGetter from in-memory kubeconfig (no temp file needed)
	getter, err := newInMemoryRESTClientGetter([]byte(kubeconfig), p.Namespace)
	if err != nil {
		return fmt.Errorf("building REST client getter: %w", err)
	}

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(getter, p.Namespace, "secrets", func(format string, v ...interface{}) {
		t.Logf("[helm] "+format, v...)
	}); err != nil {
		return fmt.Errorf("initialising helm action config: %w", err)
	}

	// Isolate Helm from the user's installed repos so that any pre-existing
	// "datadog" entry in ~/.config/helm/repositories.yaml doesn't interfere.
	// LocateChart uses RepositoryConfig to match the chart URL against known
	// repos and then loads their index from RepositoryCache; if it finds a
	// match but the index isn't there it returns "no cached repo found".
	// Pointing both to a temp dir ensures a clean slate.
	isolated := t.TempDir()
	envSettings := cli.New()
	envSettings.RepositoryCache = isolated
	envSettings.RepositoryConfig = isolated + "/repositories.yaml"

	// Check whether the release already exists so we can branch to install vs upgrade.
	// action.NewGet returns an error when the release is not found.
	releaseExists := false
	getAction := action.NewGet(actionConfig)
	if _, err := getAction.Run(defaultReleaseName); err == nil {
		releaseExists = true
	}

	// DependencyUpdate is false: charts fetched from a Helm repo already have
	// dependencies bundled in the tarball, so there is nothing to resolve.
	if !releaseExists {
		install := action.NewInstall(actionConfig)
		install.ReleaseName = defaultReleaseName
		install.Namespace = p.Namespace
		install.Version = chartVersion
		install.RepoURL = p.HelmRepoURL
		install.Wait = true
		install.Timeout = time.Duration(p.TimeoutSeconds) * time.Second
		chartPath, err := install.LocateChart(p.HelmChartPath, envSettings)
		if err != nil {
			return fmt.Errorf("locating chart %s from %s: %w", p.HelmChartPath, p.HelmRepoURL, err)
		}
		ch, err := loader.Load(chartPath)
		if err != nil {
			return fmt.Errorf("loading chart from %s: %w", chartPath, err)
		}
		rel, err := install.RunWithContext(context.Background(), ch, vals)
		if err != nil {
			return fmt.Errorf("running install: %w", err)
		}
		t.Logf("helm release %s/%s installed at revision %d", rel.Namespace, rel.Name, rel.Version)
		return nil
	}

	upgrade := action.NewUpgrade(actionConfig)
	upgrade.Namespace = p.Namespace
	upgrade.Version = chartVersion
	upgrade.RepoURL = p.HelmRepoURL
	upgrade.Wait = true
	upgrade.Timeout = time.Duration(p.TimeoutSeconds) * time.Second

	chartPath, err := upgrade.LocateChart(p.HelmChartPath, envSettings)
	if err != nil {
		return fmt.Errorf("locating chart %s from %s: %w", p.HelmChartPath, p.HelmRepoURL, err)
	}

	ch, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("loading chart from %s: %w", chartPath, err)
	}

	rel, err := upgrade.RunWithContext(context.Background(), defaultReleaseName, ch, vals)
	if err != nil {
		return fmt.Errorf("running upgrade: %w", err)
	}
	t.Logf("helm release %s/%s upgraded to revision %d", rel.Namespace, rel.Name, rel.Version)
	return nil
}

// inMemoryRESTClientGetter implements genericclioptions.RESTClientGetter using
// an in-memory kubeconfig, so no temp file is needed.
type inMemoryRESTClientGetter struct {
	restConfig *rest.Config
	kubeconfig []byte
	namespace  string
}

func newInMemoryRESTClientGetter(kubeconfig []byte, namespace string) (*inMemoryRESTClientGetter, error) {
	rc, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	return &inMemoryRESTClientGetter{restConfig: rc, kubeconfig: kubeconfig, namespace: namespace}, nil
}

func (g *inMemoryRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	return rest.CopyConfig(g.restConfig), nil
}

func (g *inMemoryRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(g.restConfig)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(dc), nil
}

func (g *inMemoryRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	dc, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(dc), nil
}

func (g *inMemoryRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	cc, err := clientcmd.NewClientConfigFromBytes(g.kubeconfig)
	if err != nil {
		// Already validated in newInMemoryRESTClientGetter; panic here is safe
		panic(fmt.Sprintf("helmagent: failed to reload kubeconfig: %v", err))
	}
	return cc
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
func buildValuesYAML(t *testing.T, env *environments.Kubernetes, p *kubernetesagentparams.Params, secretName, pullSecretName string) string {
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
    collectCrMetrics:
      - groupVersionKind:
          group: datadoghq.com
          kind: DatadogMetric
          version: v1alpha1
        commonLabels:
          cr_type: ddm
        labelsFromPath:
          ddm_namespace: [metadata, namespace]
          ddm_name: [metadata, name]
        metrics:
          - name: ddm_value
            help: DatadogMetric value
            each:
              type: gauge
              gauge:
                path: [status, currentValue]
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
  confd:
    container_image.yaml: |
      ad_identifiers:
        - _container_image
      init_config: {}
      instances:
        - periodic_refresh_seconds: 60
    sbom.yaml: |
      ad_identifiers:
        - _sbom
      init_config: {}
      instances:
        - periodic_refresh_seconds: 60
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
  useConfigMap: true
  customAgentConfig:
    metadata_providers:
      - name: host
        interval: 120
        early_interval: 60
  podAnnotations:
    ad.datadoghq.com/agent.checks: '{"openmetrics":{"init_config":{},"instances":[{"openmetrics_endpoint":"http://localhost:6000/telemetry","namespace":"datadog.agent","metrics":[".*"]}]}}'
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
    - name: DD_KUBERNETES_NAMESPACE_LABELS_AS_TAGS
      value: "{}"
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

	// Image pull secret references — only applied when a pull secret was created
	if pullSecretName != "" {
		base = mustMerge(t, base, buildPullSecretValues(pullSecretName))
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

const imagePullSecretName = "registry-credentials"

// createImagePullSecret creates a kubernetes.io/dockerconfigjson secret named
// "registry-credentials" when E2E_IMAGE_PULL_REGISTRY/USERNAME/PASSWORD are
// configured. Mirrors utils.NewImagePullSecret in common/utils/kubernetes.go.
// Returns the secret name, or "" if no registry credentials are configured.
func createImagePullSecret(ctx context.Context, k8sClient kubernetes.Interface, namespace string) (string, error) {
	profile := runner.GetProfile()

	registryStr, _ := profile.ParamStore().GetWithDefault(parameters.ImagePullRegistry, "")
	if registryStr == "" {
		return "", nil
	}

	usernameStr, _ := profile.ParamStore().GetWithDefault(parameters.ImagePullUsername, "")
	passwordStr, _ := profile.SecretStore().GetWithDefault(parameters.ImagePullPassword, "")
	if usernameStr == "" || passwordStr == "" {
		return "", fmt.Errorf("image_pull_registry is set but image_pull_username or image_pull_password is missing")
	}

	registries := strings.Split(registryStr, ",")
	usernames := strings.Split(usernameStr, ",")
	passwords := strings.Split(passwordStr, ",")

	if len(registries) != len(usernames) || len(registries) != len(passwords) {
		return "", fmt.Errorf("image_pull_registry, image_pull_username, and image_pull_password must have the same number of comma-separated entries")
	}

	authMap := make(map[string]map[string]string, len(registries))
	for i := range registries {
		pwd := strings.TrimSpace(passwords[i])
		if strings.HasPrefix(pwd, "b64=") {
			decoded, err := base64.StdEncoding.DecodeString(pwd[4:])
			if err != nil {
				return "", fmt.Errorf("failed to base64-decode password for registry %s: %w", registries[i], err)
			}
			pwd = string(decoded)
		}
		reg := strings.TrimSpace(registries[i])
		usr := strings.TrimSpace(usernames[i])
		authMap[reg] = map[string]string{
			"username": usr,
			"password": pwd,
			"auth":     base64.StdEncoding.EncodeToString([]byte(usr + ":" + pwd)),
		}
	}

	dockerConfigJSON, err := json.Marshal(map[string]map[string]map[string]string{"auths": authMap})
	if err != nil {
		return "", fmt.Errorf("failed to marshal dockerconfigjson: %w", err)
	}

	_, err = k8sClient.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: imagePullSecretName, Namespace: namespace},
		StringData: map[string]string{
			".dockerconfigjson": string(dockerConfigJSON),
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return "", fmt.Errorf("failed to create image pull secret: %w", err)
	}

	return imagePullSecretName, nil
}

// buildPullSecretValues returns Helm YAML that adds the named imagePullSecret
// to agents, clusterAgent, and clusterChecksRunner image sections.
// Mirrors configureImagePullSecret in kubernetes_helm.go.
func buildPullSecretValues(secretName string) string {
	return fmt.Sprintf(`agents:
  image:
    pullSecrets:
      - name: %[1]s
clusterAgent:
  image:
    pullSecrets:
      - name: %[1]s
clusterChecksRunner:
  image:
    pullSecrets:
      - name: %[1]s
`, secretName)
}

// buildImageValues generates Helm values for agent/cluster-agent images based
// on the runner profile (pipeline ID, commit SHA) or user-provided image paths.
// Mirrors the logic in docker_image.go's dockerAgentFullImagePath /
// dockerClusterAgentFullImagePath when no explicit image is provided.
func buildImageValues(t *testing.T, p *kubernetesagentparams.Params) string {
	t.Helper()

	// User-provided full image paths take precedence
	if p.AgentFullImagePath != "" {
		repo, tag := parseImageRef(p.AgentFullImagePath)
		containerRegistry, imageName := splitRepoForSidecar(repo)
		values := fmt.Sprintf(`agents:
  image:
    repository: %s
    tag: "%s"
    doNotCheckTag: true
clusterChecksRunner:
  image:
    repository: %s
    tag: "%s"
    doNotCheckTag: true
clusterAgent:
  admissionController:
    agentSidecarInjection:
      containerRegistry: %s
      imageName: %s
      imageTag: "%s"
`, repo, tag, repo, tag, containerRegistry, imageName, tag)
		if p.ClusterAgentFullImagePath != "" {
			cRepo, cTag := parseImageRef(p.ClusterAgentFullImagePath)
			values += fmt.Sprintf("clusterAgent:\n  image:\n    repository: %s\n    tag: \"%s\"\n    doNotCheckTag: true\n", cRepo, cTag)
		}
		return values
	}

	// Check for pipeline-based image from runner profile
	profile := runner.GetProfile()
	pipelineID, _ := profile.ParamStore().GetWithDefault(parameters.PipelineID, "")
	commitSHA, _ := profile.ParamStore().GetWithDefault(parameters.CommitSHA, "")

	if pipelineID != "" && commitSHA != "" {
		internalRegistry := runner.InternalRegistry()
		if internalRegistry == "" {
			// No internal registry configured for this environment — fall through to chart defaults
			return ""
		}

		// Images are tagged with the short (8-char) commit SHA. CI sets
		// E2E_COMMIT_SHA=$CI_COMMIT_SHORT_SHA so it is already 8 chars, but
		// truncate defensively in case a full SHA is passed locally.
		shortSHA := commitSHA
		if len(shortSHA) > 8 {
			shortSHA = shortSHA[:8]
		}

		agentTag := fmt.Sprintf("%s-%s", pipelineID, shortSHA)
		// -linux restricts to the single-platform Linux manifest. It is the
		// default; the multi-arch manifest (no -linux) is only used when
		// WindowsImage is explicitly set, mirroring docker_image.go where
		// useLinuxOnly = e.AgentLinuxOnly() && !windowsImage.
		linuxOnly := !p.WindowsImage
		switch {
		case linuxOnly && p.FIPS && p.JMX:
			agentTag += "-fips-linux-jmx"
		case linuxOnly && p.FIPS:
			agentTag += "-fips-linux"
		case p.FIPS && p.JMX:
			agentTag += "-fips-jmx"
		case p.FIPS:
			agentTag += "-fips"
		case linuxOnly && p.JMX:
			agentTag += "-linux-jmx"
		case linuxOnly:
			agentTag += "-linux"
		case p.JMX:
			agentTag += "-jmx"
		}

		clusterAgentTag := fmt.Sprintf("%s-%s", pipelineID, shortSHA)
		if p.FIPS {
			clusterAgentTag += "-fips"
		}

		return fmt.Sprintf(`agents:
  image:
    repository: %[1]s/agent-qa
    tag: "%[2]s"
    doNotCheckTag: true
clusterChecksRunner:
  image:
    repository: %[1]s/agent-qa
    tag: "%[2]s"
    doNotCheckTag: true
clusterAgent:
  image:
    repository: %[1]s/cluster-agent-qa
    tag: "%[3]s"
    doNotCheckTag: true
  admissionController:
    agentSidecarInjection:
      containerRegistry: %[1]s
      imageName: agent-qa
      imageTag: "%[2]s"
`, internalRegistry, agentTag, clusterAgentTag)
	}

	// No specific image — use chart defaults (latest stable)
	return ""
}

// splitRepoForSidecar splits a full repository path into the registry prefix
// and the image name, for use in agentSidecarInjection.containerRegistry and
// imageName. Mirrors the split in buildLinuxHelmValues in kubernetes_helm.go.
// e.g. "669783387624.dkr.ecr.us-east-1.amazonaws.com/agent-qa"
//   → ("669783387624.dkr.ecr.us-east-1.amazonaws.com", "agent-qa")
func splitRepoForSidecar(repo string) (containerRegistry, imageName string) {
	if idx := strings.LastIndex(repo, "/"); idx != -1 {
		return repo[:idx], repo[idx+1:]
	}
	return "", repo
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

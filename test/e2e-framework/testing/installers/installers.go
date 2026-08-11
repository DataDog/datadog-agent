// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installers installs/upgrades the Datadog Agent into an
// environment described by an envctl-style JSON file, in-process (no
// subprocess). It backs the `envctl install-agent` CLI command, and is
// also called directly by tests that need to change agent config
// mid-suite (see test/new-e2e/examples/kind_nopulumi_test.go) — both
// callers share this one code path instead of one of them shelling out
// to the other.
package installers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v2"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	compagent "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	compkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	runnerparams "github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// InstallParams carries the caller-level, environment-agnostic install
// request. Concrete installers pull whatever environment-specific
// resources they need out of the env file's entries themselves.
type InstallParams struct {
	AgentVersion        string
	ClusterAgentVersion string
	Namespace           string
	APIKey, AppKey      string
}

// InstallStatus is an installer's read of its own "agent" state-file entry
// relative to a desired InstallParams — e.g. "installed and matches" vs.
// "installed but the YAML now wants a different version."
type InstallStatus struct {
	Summary  string
	UpToDate bool
}

// Installer installs a given agent version into an already-provisioned
// environment and returns the "agent" entry to merge back into the
// environment file.
type Installer interface {
	// detect reports whether this installer recognizes the given
	// environment description (e.g. a "kubernetesCluster" entry).
	detect(envEntries map[string]json.RawMessage) bool
	install(ctx context.Context, envEntries map[string]json.RawMessage, p InstallParams) (json.RawMessage, error)
	// status compares agentRaw (this installer's own previously-returned
	// "agent" entry) against desired, reporting a human-readable summary
	// plus whether they match.
	status(agentRaw json.RawMessage, desired InstallParams) (InstallStatus, error)
}

var registry = map[string]Installer{
	"helm-k8s": helmKubernetesInstaller{},
}

// resolve picks the named installer, or — if name is empty — auto-detects
// one by asking each registered installer whether it recognizes the
// environment description.
func resolve(name string, envEntries map[string]json.RawMessage) (Installer, error) {
	if name != "" {
		inst, ok := registry[name]
		if !ok {
			return nil, fmt.Errorf("unknown installer %q", name)
		}
		return inst, nil
	}
	for _, inst := range registry {
		if inst.detect(envEntries) {
			return inst, nil
		}
	}
	return nil, errors.New("could not auto-detect an installer for this environment; pass an installer name explicitly")
}

// UpdateAgent installs or upgrades the agent into the environment
// described by envPath, merging the result back into that file — the
// operation `envctl install-agent` performs, exposed as a plain function
// so callers (this CLI, or a running test via BaseSuite.UpdateEnv) can
// invoke it in-process. installerName selects the installer explicitly;
// pass "" to auto-detect from the environment description.
func UpdateAgent(ctx context.Context, envPath, installerName string, params InstallParams) error {
	entries, err := readEnvFile(envPath)
	if err != nil {
		return err
	}

	inst, err := resolve(installerName, entries)
	if err != nil {
		return err
	}

	agentJSON, err := inst.install(ctx, entries, params)
	if err != nil {
		return err
	}

	entries["agent"] = agentJSON
	return writeEnvFileAtomic(envPath, entries)
}

// Status reports the installed agent's status relative to desired,
// resolving the installer the same way UpdateAgent does (installerName
// explicit, or auto-detected from envEntries). Returns "not installed"
// directly, with no installer resolution, if envEntries has no "agent"
// entry yet.
func Status(installerName string, envEntries map[string]json.RawMessage, desired InstallParams) (InstallStatus, error) {
	raw, ok := envEntries["agent"]
	if !ok {
		return InstallStatus{Summary: "not installed"}, nil
	}
	inst, err := resolve(installerName, envEntries)
	if err != nil {
		return InstallStatus{}, err
	}
	return inst.status(raw, desired)
}

// ResolveAPIKeys mirrors the two sources the existing local dev profile
// already reads (testing/runner/local_profile.go): E2E_API_KEY/E2E_APP_KEY
// env vars first, then ~/.test_infra_config.yaml — read directly here
// instead of via config.Env.AgentAPIKey(), which needs a pulumi.Context.
func ResolveAPIKeys() (apiKey, appKey string, err error) {
	apiKey = os.Getenv("E2E_API_KEY")
	appKey = os.Getenv("E2E_APP_KEY")
	if apiKey != "" && appKey != "" {
		return apiKey, appKey, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	data, readErr := os.ReadFile(filepath.Join(home, ".test_infra_config.yaml"))
	if readErr != nil {
		if apiKey == "" {
			return "", "", fmt.Errorf("no E2E_API_KEY set and could not read ~/.test_infra_config.yaml: %w", readErr)
		}
		return apiKey, appKey, nil
	}

	var cfg runnerparams.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", "", err
	}
	if apiKey == "" {
		apiKey = cfg.ConfigParams.Agent.APIKey
	}
	if appKey == "" {
		appKey = cfg.ConfigParams.Agent.APPKey
	}
	if apiKey == "" {
		return "", "", errors.New("no API key found (set E2E_API_KEY or ~/.test_infra_config.yaml configParams.agent.apiKey)")
	}
	return apiKey, appKey, nil
}

// --- environment file helpers -------------------------------------------

func readEnvFile(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return entries, nil
}

// writeEnvFileAtomic writes entries to path via a temp file + rename, so a
// crash mid-write never leaves a corrupted/partial environment file behind.
func writeEnvFileAtomic(path string, entries map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// --- helm-k8s installer ---------------------------------------------------

// helmKubernetesInstaller installs the datadog Helm chart into a
// Kubernetes environment — however it was provisioned (kind, EKS, or
// otherwise), so long as the environment file has a "kubernetesCluster"
// entry shaped like components/kubernetes.ClusterOutput.
type helmKubernetesInstaller struct{}

// matches the "dda-linux-datadog*" label convention helm.NewKubernetesAgent uses
const helmReleaseName = "dda-linux"

func (helmKubernetesInstaller) detect(envEntries map[string]json.RawMessage) bool {
	_, ok := envEntries["kubernetesCluster"]
	return ok
}

func (helmKubernetesInstaller) install(_ context.Context, envEntries map[string]json.RawMessage, p InstallParams) (json.RawMessage, error) {
	clusterRaw, ok := envEntries["kubernetesCluster"]
	if !ok {
		return nil, errors.New(`no "kubernetesCluster" entry in the environment file`)
	}
	var cluster compkube.ClusterOutput
	if err := json.Unmarshal(clusterRaw, &cluster); err != nil {
		return nil, err
	}

	var fi *fakeintake.FakeintakeOutput
	if fiRaw, ok := envEntries["fakeIntake"]; ok {
		fi = &fakeintake.FakeintakeOutput{}
		if err := json.Unmarshal(fiRaw, fi); err != nil {
			return nil, err
		}
	}

	kubeconfigPath, cleanup, err := writeTempFile("kubeconfig-*.yaml", cluster.KubeConfig)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	flags := genericclioptions.NewConfigFlags(false)
	flags.KubeConfig = &kubeconfigPath
	flags.Namespace = &p.Namespace

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(flags, p.Namespace, "secret", func(string, ...interface{}) {}); err != nil {
		return nil, err
	}

	clusterAgentToken, err := randomHex(16)
	if err != nil {
		return nil, err
	}

	values := buildHelmValues(p, cluster.ClusterName, fi, clusterAgentToken)
	if err := installOrUpgradeHelmRelease(actionConfig, helmReleaseName, p.Namespace, values); err != nil {
		return nil, err
	}

	// LinuxClusterChecks intentionally left unset (zero value): this POC's
	// values don't enable clusterChecksRunner, so no pods would ever match
	// a "dda-linux-datadog-clusterchecks" selector.
	agentOutput := compagent.KubernetesAgentOutput{
		LinuxNodeAgent: compkube.KubernetesObjRefOutput{
			Namespace:      p.Namespace,
			Kind:           "Pod",
			Version:        p.AgentVersion,
			LabelSelectors: map[string]string{"app": helmReleaseName + "-datadog"},
		},
		LinuxClusterAgent: compkube.KubernetesObjRefOutput{
			Namespace:      p.Namespace,
			Kind:           "Pod",
			Version:        p.ClusterAgentVersion,
			LabelSelectors: map[string]string{"app": helmReleaseName + "-datadog-cluster-agent"},
		},
	}
	return json.Marshal(agentOutput)
}

func (helmKubernetesInstaller) status(agentRaw json.RawMessage, desired InstallParams) (InstallStatus, error) {
	var got compagent.KubernetesAgentOutput
	if err := json.Unmarshal(agentRaw, &got); err != nil {
		return InstallStatus{}, fmt.Errorf("parsing recorded agent entry: %w", err)
	}

	upToDate := got.LinuxNodeAgent.Version == desired.AgentVersion &&
		got.LinuxClusterAgent.Version == desired.ClusterAgentVersion &&
		got.LinuxNodeAgent.Namespace == desired.Namespace

	installed := fmt.Sprintf("agent %s / cluster-agent %s (namespace %s)",
		got.LinuxNodeAgent.Version, got.LinuxClusterAgent.Version, got.LinuxNodeAgent.Namespace)
	if upToDate {
		return InstallStatus{Summary: installed + " (up to date)", UpToDate: true}, nil
	}

	wanted := fmt.Sprintf("agent %s / cluster-agent %s (namespace %s)",
		desired.AgentVersion, desired.ClusterAgentVersion, desired.Namespace)
	return InstallStatus{
		Summary: fmt.Sprintf("%s installed, YAML wants %s — drifted", installed, wanted),
	}, nil
}

func installOrUpgradeHelmRelease(cfg *action.Configuration, releaseName, namespace string, values map[string]interface{}) error {
	settings := cli.New()

	chartPathOpts := action.ChartPathOptions{RepoURL: "https://helm.datadoghq.com"}
	chartPath, err := chartPathOpts.LocateChart("datadog", settings)
	if err != nil {
		return fmt.Errorf("locating datadog chart: %w", err)
	}
	chartRequested, err := loader.Load(chartPath)
	if err != nil {
		return err
	}

	history := action.NewHistory(cfg)
	history.Max = 1
	if _, err := history.Run(releaseName); errors.Is(err, driver.ErrReleaseNotFound) {
		install := action.NewInstall(cfg)
		install.ChartPathOptions = chartPathOpts
		install.ReleaseName = releaseName
		install.Namespace = namespace
		install.CreateNamespace = true
		install.Wait = true
		install.Timeout = 5 * time.Minute
		_, err := install.Run(chartRequested, values)
		return err
	} else if err != nil {
		return err
	}

	upgrade := action.NewUpgrade(cfg)
	upgrade.ChartPathOptions = chartPathOpts
	upgrade.Namespace = namespace
	upgrade.Wait = true
	upgrade.Timeout = 5 * time.Minute
	_, err = upgrade.Run(releaseName, chartRequested, values)
	return err
}

// buildHelmValues is a deliberately minimal, plain-Go values builder — NOT
// a port of buildLinuxHelmValues (components/datadog/agent/kubernetes_helm.go),
// which returns a pulumi.Map and depends on genuine async Pulumi Outputs.
// Kube-state-metrics, SBOM, autoscaling, APM instrumentation, OTel, Windows,
// FIPS, JMX are all out of scope for this POC.
func buildHelmValues(p InstallParams, clusterName string, fi *fakeintake.FakeintakeOutput, clusterAgentToken string) map[string]interface{} {
	datadog := map[string]interface{}{
		"apiKey":      p.APIKey,
		"appKey":      p.AppKey,
		"clusterName": clusterName,
		"kubelet": map[string]interface{}{
			"tlsVerify": false,
		},
	}
	if fi != nil {
		datadog["dd_url"] = fi.URL
		datadog["skipSslValidation"] = true
	}

	return map[string]interface{}{
		"datadog": datadog,
		"agents": map[string]interface{}{
			"useHostNetwork": true,
			"image":          map[string]interface{}{"tag": p.AgentVersion},
		},
		"clusterAgent": map[string]interface{}{
			"token": clusterAgentToken,
			"image": map[string]interface{}{"tag": p.ClusterAgentVersion},
		},
	}
}

func writeTempFile(pattern, content string) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	f.Close()
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

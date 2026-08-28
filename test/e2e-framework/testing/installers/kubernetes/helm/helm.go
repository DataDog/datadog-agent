// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package helm installs the Agent in a Kubernetes environment with the Datadog Helm chart.
package helm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	helmaction "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	compagent "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	compkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// Params configures a Helm-based Kubernetes Agent installation.
type Params struct {
	AgentVersion        string
	ClusterAgentVersion string
	Namespace           string
	// Values, if set, is deep-merged over the chart's default values.
	Values map[string]interface{}
}

// Install installs or upgrades the Datadog Helm chart in env and updates
// env.Agent to describe the installed workloads. It only relies on env's
// initialized components and is independent of the provisioner that created it.
func Install(_ context.Context, env *environments.Kubernetes, p Params) error {
	if env == nil || env.KubernetesCluster == nil {
		return errors.New("installing Kubernetes agent: environment's KubernetesCluster is not initialized")
	}

	secretStore := runner.GetProfile().SecretStore()
	apiKey, err := secretStore.Get(parameters.APIKey)
	if err != nil {
		return fmt.Errorf("resolving Agent API key: %w", err)
	}
	appKey, err := secretStore.Get(parameters.APPKey)
	if err != nil {
		return fmt.Errorf("resolving Agent application key: %w", err)
	}

	var fi *fakeintake.FakeintakeOutput
	var rcRootJSON string
	if env.FakeIntake != nil {
		fi = &env.FakeIntake.FakeintakeOutput
		rcRootJSON, err = fakeintake.RCRootJSON()
		if err != nil {
			return fmt.Errorf("building fakeintake Remote Config root: %w", err)
		}
	}

	output, err := installChart(env.KubernetesCluster.KubeConfig, env.KubernetesCluster.ClusterName, fi, chartParams{
		AgentVersion:        p.AgentVersion,
		ClusterAgentVersion: p.ClusterAgentVersion,
		Namespace:           p.Namespace,
		APIKey:              apiKey,
		AppKey:              appKey,
		RCRootJSON:          rcRootJSON,
		Values:              p.Values,
	})
	if err != nil {
		return err
	}

	if env.Agent == nil {
		env.Agent = &components.KubernetesAgent{}
	}
	env.Agent.KubernetesAgentOutput = output
	return nil
}

const releaseName = "dda-linux"

type chartParams struct {
	AgentVersion, ClusterAgentVersion, Namespace string
	APIKey, AppKey, RCRootJSON                   string
	Values                                       map[string]interface{}
}

func installChart(kubeconfig, clusterName string, fi *fakeintake.FakeintakeOutput, p chartParams) (compagent.KubernetesAgentOutput, error) {
	kubeconfigPath, cleanup, err := writeTempFile("kubeconfig-*.yaml", kubeconfig)
	if err != nil {
		return compagent.KubernetesAgentOutput{}, err
	}
	defer cleanup()

	flags := genericclioptions.NewConfigFlags(false)
	flags.KubeConfig = &kubeconfigPath
	flags.Namespace = &p.Namespace

	actionConfig := new(helmaction.Configuration)
	if err := actionConfig.Init(flags, p.Namespace, "secret", func(string, ...interface{}) {}); err != nil {
		return compagent.KubernetesAgentOutput{}, err
	}

	clusterAgentToken, err := randomHex(16)
	if err != nil {
		return compagent.KubernetesAgentOutput{}, err
	}

	values := buildValues(p, clusterName, fi, clusterAgentToken)
	if p.Values != nil {
		mergeMaps(values, p.Values)
	}
	if err := installOrUpgradeRelease(actionConfig, releaseName, p.Namespace, values); err != nil {
		return compagent.KubernetesAgentOutput{}, err
	}

	return compagent.KubernetesAgentOutput{
		LinuxNodeAgent: compkube.KubernetesObjRefOutput{
			Namespace:      p.Namespace,
			Kind:           "Pod",
			Version:        p.AgentVersion,
			LabelSelectors: map[string]string{"app": releaseName + "-datadog"},
		},
		LinuxClusterAgent: compkube.KubernetesObjRefOutput{
			Namespace:      p.Namespace,
			Kind:           "Pod",
			Version:        p.ClusterAgentVersion,
			LabelSelectors: map[string]string{"app": releaseName + "-datadog-cluster-agent"},
		},
	}, nil
}

func mergeMaps(dst, src map[string]interface{}) {
	for k, v := range src {
		if srcMap, ok := v.(map[string]interface{}); ok {
			if dstMap, ok := dst[k].(map[string]interface{}); ok {
				mergeMaps(dstMap, srcMap)
				continue
			}
		}
		dst[k] = v
	}
}

func installOrUpgradeRelease(cfg *helmaction.Configuration, name, namespace string, values map[string]interface{}) error {
	settings := cli.New()

	chartPathOpts := helmaction.ChartPathOptions{RepoURL: "https://helm.datadoghq.com"}
	chartPath, err := chartPathOpts.LocateChart("datadog", settings)
	if err != nil {
		return fmt.Errorf("locating datadog chart: %w", err)
	}
	chartRequested, err := loader.Load(chartPath)
	if err != nil {
		return err
	}

	history := helmaction.NewHistory(cfg)
	history.Max = 1
	if _, err := history.Run(name); errors.Is(err, driver.ErrReleaseNotFound) {
		install := helmaction.NewInstall(cfg)
		install.ChartPathOptions = chartPathOpts
		install.ReleaseName = name
		install.Namespace = namespace
		install.CreateNamespace = true
		install.Wait = true
		install.Timeout = 5 * time.Minute
		_, err := install.Run(chartRequested, values)
		return err
	} else if err != nil {
		return err
	}

	upgrade := helmaction.NewUpgrade(cfg)
	upgrade.ChartPathOptions = chartPathOpts
	upgrade.Namespace = namespace
	upgrade.Wait = true
	upgrade.Timeout = 5 * time.Minute
	_, err = upgrade.Run(name, chartRequested, values)
	return err
}

// buildValues is deliberately minimal and independent of Pulumi Outputs.
// Kube-state-metrics, SBOM, autoscaling, APM instrumentation, OTel, Windows,
// FIPS, and JMX are out of scope for this POC.
func buildValues(p chartParams, clusterName string, fi *fakeintake.FakeintakeOutput, clusterAgentToken string) map[string]interface{} {
	datadog := map[string]interface{}{
		"apiKey":      p.APIKey,
		"appKey":      p.AppKey,
		"clusterName": clusterName,
		"kubelet": map[string]interface{}{
			"tlsVerify": false,
		},
	}
	clusterAgent := map[string]interface{}{
		"token": clusterAgentToken,
		"image": map[string]interface{}{"tag": p.ClusterAgentVersion},
	}
	if fi != nil {
		datadog["dd_url"] = fi.URL
		datadog["skipSslValidation"] = true

		rcEnv := []interface{}{
			map[string]interface{}{"name": "DD_REMOTE_CONFIGURATION_RC_DD_URL", "value": fi.URL},
			map[string]interface{}{"name": "DD_REMOTE_CONFIGURATION_NO_TLS", "value": "true"},
			map[string]interface{}{"name": "DD_REMOTE_CONFIGURATION_NO_TLS_VALIDATION", "value": "true"},
			map[string]interface{}{"name": "DD_REMOTE_CONFIGURATION_CONFIG_ROOT", "value": p.RCRootJSON},
			map[string]interface{}{"name": "DD_REMOTE_CONFIGURATION_DIRECTOR_ROOT", "value": p.RCRootJSON},
			map[string]interface{}{"name": "DD_REMOTE_CONFIGURATION_REFRESH_INTERVAL", "value": "5s"},
		}
		datadog["env"] = rcEnv
		clusterAgent["env"] = rcEnv
	}

	return map[string]interface{}{
		"datadog": datadog,
		"agents": map[string]interface{}{
			"useHostNetwork": true,
			"image":          map[string]interface{}{"tag": p.AgentVersion},
		},
		"clusterAgent": clusterAgent,
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

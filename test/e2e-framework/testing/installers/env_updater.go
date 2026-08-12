// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// HelmK8sInstallParams is helm-k8s's own update-parameters shape — deliberately not
// shared with any other installer/environment type: ClusterAgentVersion/Namespace/
// HelmValues are specific to a Helm-deployed Kubernetes agent and wouldn't make sense
// on, say, a Host installer's params.
type HelmK8sInstallParams struct {
	AgentVersion        string
	ClusterAgentVersion string
	Namespace           string
	// HelmValues, if set, is deep-merged over the chart's default values.
	HelmValues map[string]interface{}
}

// InstallHelmK8s installs or upgrades the datadog Helm chart using cluster's (and, if
// enabled, fakeIntake's) already-live connection info — works whether they came from a
// real Pulumi apply or from a SingleFileProvisioner-backed state file, since both paths
// call the same component Init(). API keys are resolved the same way ResolveAPIKeys
// always has; callers never supply them. Used by environments.Kubernetes.UpdateAgent.
func InstallHelmK8s(_ context.Context, cluster *components.KubernetesCluster, fakeIntake *components.FakeIntake, p HelmK8sInstallParams) (json.RawMessage, error) {
	apiKey, appKey, err := ResolveAPIKeys()
	if err != nil {
		return nil, err
	}
	var fi *fakeintake.FakeintakeOutput
	if fakeIntake != nil {
		fi = &fakeIntake.FakeintakeOutput
	}
	return installHelmChart(cluster.KubeConfig, cluster.ClusterName, fi, helmChartParams{
		AgentVersion:        p.AgentVersion,
		ClusterAgentVersion: p.ClusterAgentVersion,
		Namespace:           p.Namespace,
		APIKey:              apiKey,
		AppKey:              appKey,
		HelmValues:          p.HelmValues,
	})
}

// HostInstallParams is the host installer's own update-parameters shape — no
// ClusterAgentVersion/Namespace/HelmValues here since none of those apply to a plain
// host install.
type HostInstallParams struct {
	AgentVersion string
}

// InstallHostAgent runs the official install script over host's already-connected SSH
// client (host.Execute) — the same script Pulumi's own host installer uses
// (components/datadog/agent/host_linuxos.go's getInstallCommand), driven by
// DD_API_KEY/DD_AGENT_MAJOR_VERSION/DD_AGENT_MINOR_VERSION env vars derived from
// p.AgentVersion. Works whether host came from a real Pulumi apply or a state file,
// since both call the same RemoteHost.Init.
func InstallHostAgent(_ context.Context, host *components.RemoteHost, p HostInstallParams) (json.RawMessage, error) {
	apiKey, _, err := ResolveAPIKeys()
	if err != nil {
		return nil, err
	}
	cmd := hostInstallScriptCommand(p.AgentVersion, apiKey)
	if _, err := host.Execute(cmd); err != nil {
		return nil, fmt.Errorf("running agent install script: %w", err)
	}
	return json.Marshal(agent.HostAgentOutput{Host: host.HostOutput})
}

// hostInstallScriptCommand builds the same curl-pipe-to-bash install command
// components/datadog/agent/host_linuxos.go's getInstallCommand constructs for a real
// Pulumi-provisioned host, as a plain string (no pulumi.StringOutput wrapping needed
// here). version == "" or "latest" omits DD_AGENT_MAJOR_VERSION/DD_AGENT_MINOR_VERSION
// entirely, letting the script default to the latest stable major 7 release.
func hostInstallScriptCommand(version, apiKey string) string {
	major := "7"
	envVars := []string{fmt.Sprintf("DD_API_KEY=%s", apiKey), "DD_INSTALL_ONLY=true"}
	if m, minor, ok := splitAgentVersion(version); ok {
		major = m
		envVars = append(envVars, fmt.Sprintf("DD_AGENT_MAJOR_VERSION=%s", major))
		if minor != "" {
			envVars = append(envVars, fmt.Sprintf("DD_AGENT_MINOR_VERSION=%s", minor))
		}
	}
	return fmt.Sprintf(
		`for i in 1 2 3 4 5; do curl -fsSL https://s3.amazonaws.com/dd-agent/scripts/install_script_agent%s.sh -o install-script.sh && break || sleep $((2**$i)); done && for i in 1 2 3; do %s bash install-script.sh && exit 0 || sleep $((2**$i)); done; exit 1`,
		major, strings.Join(envVars, " "))
}

// splitAgentVersion mirrors the "7."/"6." major-prefix convention
// components/datadog/agentparams.parseVersion already uses to build a PackageVersion
// from a version string — duplicated here in miniature (that function is unexported,
// in an unrelated package, and returns a whole PackageVersion when all we need is the
// major/minor split). ok is false for "" or "latest", where the caller omits both env
// vars and lets the install script pick its own default.
func splitAgentVersion(version string) (major, minor string, ok bool) {
	for _, m := range []string{"7", "6"} {
		if minor, ok := strings.CutPrefix(version, m+"."); ok {
			return m, minor, true
		}
	}
	return "", "", false
}

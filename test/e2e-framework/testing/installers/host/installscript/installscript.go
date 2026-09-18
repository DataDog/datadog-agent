// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installscript installs the Agent on a host with the official install script.
package installscript

import (
	"context"
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/host/configure"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentconfig"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// Params configures an install-script Agent installation.
type Params struct {
	Routing      *receivers.Plan
	APIKey       string // transient native credential; never needed for capture/sink
	AgentVersion string
	// AgentConfig is merged over the installer's generated datadog.yaml.
	AgentConfig string
	// Integrations maps conf.d folder names (for example, "custom_logs.d")
	// to their conf.yaml contents.
	Integrations map[string]string
}

// Install runs the official install script on env.RemoteHost and updates env.Agent.
// It only relies on env's initialized components and is independent of the
// provisioner that created the environment.
func Install(_ context.Context, env *environments.Host, p Params) error {
	if env == nil || env.RemoteHost == nil {
		return errors.New("installing host agent: environment's RemoteHost is not initialized")
	}

	if p.Routing != nil && p.AgentVersion != "7.83.0" {
		return fmt.Errorf("install-script released routing profile supports Agent 7.83.0 only")
	}

	apiKey := p.APIKey
	var config string
	var err error
	if p.Routing != nil {
		if p.Routing.APIKeyRef == "" {
			apiKey = receivers.DummyAPIKey
		}
		config, err = agentconfig.GenerateWithRouting(*p.Routing, apiKey, p.AgentConfig)
	} else {
		apiKey, err = runner.GetProfile().SecretStore().Get(parameters.APIKey)
		if err != nil {
			return fmt.Errorf("resolving Agent API key: %w", err)
		}
		config, err = buildAgentConfig(env, apiKey, p.AgentConfig)
	}
	if err != nil {
		return err
	}

	if err := configure.ValidateIntegrations(p.Integrations); err != nil {
		return err
	}

	// Install-only must never carry the real key in a command or start a native sender.
	cmd := command(p.AgentVersion, receivers.DummyAPIKey)
	if _, err := env.RemoteHost.Execute(cmd); err != nil {
		return fmt.Errorf("running agent install script: %w", err)
	}
	return configure.Apply(env, config, p.Integrations)
}

// command builds the same curl-pipe-to-bash install command
// components/datadog/agent/host_linuxos.go's getInstallCommand constructs for a real
// Pulumi-provisioned host. version == "" or "latest" omits the version variables,
// letting the script default to the latest stable major 7 release.
func command(version, apiKey string) string {
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

// buildAgentConfig is retained for the existing script renderer tests.
func buildAgentConfig(env *environments.Host, apiKey, extra string) (string, error) {
	return configure.Generate(env, apiKey, extra)
}

func splitAgentVersion(version string) (major, minor string, ok bool) {
	for _, m := range []string{"7", "6"} {
		if minor, ok := strings.CutPrefix(version, m+"."); ok {
			return m, minor, true
		}
	}
	return "", "", false
}

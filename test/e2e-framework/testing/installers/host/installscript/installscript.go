// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installscript installs the Agent on a host with the official install script.
package installscript

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// Params configures an install-script Agent installation.
type Params struct {
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

	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	if err != nil {
		return fmt.Errorf("resolving Agent API key: %w", err)
	}
	config, err := buildAgentConfig(env, apiKey, p.AgentConfig)
	if err != nil {
		return err
	}
	for folder := range p.Integrations {
		if !integrationFolderPattern.MatchString(folder) {
			return fmt.Errorf("invalid integration folder %q", folder)
		}
	}

	cmd := command(p.AgentVersion, apiKey)
	if _, err := env.RemoteHost.Execute(cmd); err != nil {
		return fmt.Errorf("running agent install script: %w", err)
	}
	if err := writeRemoteFile(env, "/etc/datadog-agent/datadog.yaml", config); err != nil {
		return err
	}
	for folder, content := range p.Integrations {
		configDir := "/etc/datadog-agent/conf.d/" + folder
		if _, err := env.RemoteHost.Execute("sudo mkdir -p " + configDir); err != nil {
			return fmt.Errorf("creating integration directory %s: %w", configDir, err)
		}
		if err := writeRemoteFile(env, configDir+"/conf.yaml", content); err != nil {
			return err
		}
	}
	if _, err := env.RemoteHost.Execute("sudo systemctl restart datadog-agent"); err != nil {
		return fmt.Errorf("restarting Agent: %w", err)
	}

	if env.Agent == nil {
		env.Agent = &components.RemoteHostAgent{}
	}
	env.Agent.HostAgentOutput = agent.HostAgentOutput{Host: env.RemoteHost.HostOutput}
	if err := env.Agent.InitFromHost(env.RemoteHost); err != nil {
		return fmt.Errorf("initializing installed Agent: %w", err)
	}
	return nil
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

var integrationFolderPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func buildAgentConfig(env *environments.Host, apiKey, extraConfig string) (string, error) {
	config := fmt.Sprintf("api_key: %q\n", apiKey)
	if env.FakeIntake != nil {
		config += fmt.Sprintf(`dd_url: %s://%s:%d
logs_config.logs_dd_url: %s:%d
logs_config.logs_no_ssl: true
logs_config.force_use_http: true
`, env.FakeIntake.Scheme, env.FakeIntake.Host, env.FakeIntake.Port, env.FakeIntake.Host, env.FakeIntake.Port)
	}
	if extraConfig == "" {
		return config, nil
	}
	merged, err := utils.MergeYAMLWithSlices(config, extraConfig)
	if err != nil {
		return "", fmt.Errorf("merging Agent config: %w", err)
	}
	return merged, nil
}

func writeRemoteFile(env *environments.Host, filePath, content string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	cmd := fmt.Sprintf("printf '%%s' '%s' | base64 --decode | sudo tee %s >/dev/null", encoded, filePath)
	if _, err := env.RemoteHost.Execute(cmd); err != nil {
		return fmt.Errorf("writing %s: %w", filePath, err)
	}
	return nil
}

func splitAgentVersion(version string) (major, minor string, ok bool) {
	for _, m := range []string{"7", "6"} {
		if minor, ok := strings.CutPrefix(version, m+"."); ok {
			return m, minor, true
		}
	}
	return "", "", false
}

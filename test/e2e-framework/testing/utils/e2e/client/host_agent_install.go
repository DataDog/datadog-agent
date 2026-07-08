// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package client

import (
	"fmt"
	"path"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	oscomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

// HostAgentInstaller installs and configures the Datadog Agent on a host over SSH, without
// Pulumi. It reuses the host's existing SSH primitives (Execute, WriteFile) and the same
// install-command builder as the Pulumi path (agent.LinuxInstallCommand), so the two cannot
// drift. Linux is supported today; Windows/macOS are follow-ups.
type HostAgentInstaller struct {
	host *Host
}

// AgentInstaller returns an installer that manages the Datadog Agent on this host over SSH.
func (h *Host) AgentInstaller() *HostAgentInstaller {
	return &HostAgentInstaller{host: h}
}

// Install installs the Agent described by params and writes its configuration, then restarts it
// so the configuration takes effect. apiKey is inlined into the install command and merged into
// datadog.yaml (unless params.SkipAPIKeyInConfig). This mirrors the Pulumi installAgent flow
// (install → write config → restart) but runs entirely over SSH.
func (i *HostAgentInstaller) Install(params *agentparams.Params, apiKey string) error {
	if i.host.osFamily != oscomp.LinuxFamily {
		return fmt.Errorf("HostAgentInstaller supports Linux only for now, got OS family %v", i.host.osFamily)
	}

	installCmd := agent.LinuxInstallCommand(i.host.arch, params.Version, apiKey, params.AdditionalInstallParameters)
	if _, err := i.host.Execute(installCmd); err != nil {
		return fmt.Errorf("agent install command failed: %w", err)
	}

	if err := i.writeConfigs(params, apiKey); err != nil {
		return err
	}

	return i.Restart()
}

// Restart restarts the Datadog Agent service.
func (i *HostAgentInstaller) Restart() error {
	if _, err := i.host.Execute("sudo systemctl restart datadog-agent"); err != nil {
		return fmt.Errorf("failed to restart datadog-agent: %w", err)
	}
	return nil
}

// writeConfigs writes datadog.yaml (merged with the API key and extra config), the system-probe
// and security-agent configs, integration configs, and any extra files declared in params.
func (i *HostAgentInstaller) writeConfigs(params *agentparams.Params, apiKey string) error {
	configDir, err := i.host.GetAgentConfigFolder()
	if err != nil {
		return err
	}

	datadogYAML, err := mergeDatadogConfig(params, apiKey)
	if err != nil {
		return err
	}

	// Core config files live under the agent config folder and must be readable by the dd-agent user.
	for _, f := range []struct{ name, content string }{
		{"datadog.yaml", datadogYAML},
		{"system-probe.yaml", params.SystemProbeConfig},
		{"security-agent.yaml", params.SecurityAgentConfig},
	} {
		if err := i.writeAgentOwnedFile(path.Join(configDir, f.name), f.content); err != nil {
			return err
		}
	}

	// Integration configs are relative to the agent config folder (e.g. conf.d/nginx.d/conf.yaml).
	for relPath, def := range params.Integrations {
		if err := i.writeFileDefinition(path.Join(configDir, relPath), def); err != nil {
			return err
		}
	}

	// Extra files use absolute paths.
	for absPath, def := range params.Files {
		if err := i.writeFileDefinition(absPath, def); err != nil {
			return err
		}
	}

	return nil
}

// writeAgentOwnedFile writes content to destPath as root and hands ownership to the dd-agent user,
// matching where the Pulumi path places core agent config.
func (i *HostAgentInstaller) writeAgentOwnedFile(destPath, content string) error {
	if err := i.stageAndInstallFile(destPath, content, true); err != nil {
		return err
	}
	if _, err := i.host.Execute(fmt.Sprintf("sudo chown dd-agent:dd-agent %s", destPath)); err != nil {
		return fmt.Errorf("failed to chown %s: %w", destPath, err)
	}
	return nil
}

// writeFileDefinition writes a single FileDefinition (integration config or extra file), honoring
// its UseSudo flag and optional file permissions.
func (i *HostAgentInstaller) writeFileDefinition(destPath string, def *agentparams.FileDefinition) error {
	if err := i.stageAndInstallFile(destPath, def.Content, def.UseSudo); err != nil {
		return err
	}
	if p, ok := def.Permissions.Get(); ok {
		if cmd := p.SetupPermissionsCommand(destPath); cmd != "" {
			if _, err := i.host.Execute(cmd); err != nil {
				return fmt.Errorf("failed to set permissions on %s: %w", destPath, err)
			}
		}
	}
	return nil
}

// stageAndInstallFile writes content to destPath. When privileged is true it stages the file in a
// user-writable temp location (via SFTP) and moves it into place with sudo, so root-owned
// destinations like /etc/datadog-agent work without embedding the content in a shell command
// (which would break on quoting).
func (i *HostAgentInstaller) stageAndInstallFile(destPath, content string, privileged bool) error {
	if !privileged {
		if _, err := i.host.WriteFile(destPath, []byte(content)); err != nil {
			return fmt.Errorf("failed to write %s: %w", destPath, err)
		}
		return nil
	}

	tmpPath := path.Join("/tmp", "e2e-agent-install-"+strings.ReplaceAll(strings.TrimPrefix(destPath, "/"), "/", "_"))
	if _, err := i.host.WriteFile(tmpPath, []byte(content)); err != nil {
		return fmt.Errorf("failed to stage %s: %w", destPath, err)
	}
	cmd := fmt.Sprintf("sudo mkdir -p %s && sudo cp %s %s && rm -f %s", path.Dir(destPath), tmpPath, destPath, tmpPath)
	if _, err := i.host.Execute(cmd); err != nil {
		return fmt.Errorf("failed to install %s: %w", destPath, err)
	}
	return nil
}

// mergeDatadogConfig builds the datadog.yaml content the same way the Pulumi path does: the base
// AgentConfig, then each extra config, then the API key (unless skipped), each merged in turn.
func mergeDatadogConfig(params *agentparams.Params, apiKey string) (string, error) {
	extras, err := resolveExtraAgentConfig(params.ExtraAgentConfig)
	if err != nil {
		return "", err
	}
	if !params.SkipAPIKeyInConfig {
		extras = append(extras, fmt.Sprintf("api_key: %s", apiKey))
	}

	merged := params.AgentConfig
	for _, extra := range extras {
		merged, err = utils.MergeYAMLWithSlices(merged, extra)
		if err != nil {
			return "", err
		}
	}
	return merged, nil
}

// resolveExtraAgentConfig turns the Pulumi-typed ExtraAgentConfig into plain strings. Only
// constant values (pulumi.String) can be resolved without a Pulumi context; computed values
// (e.g. fakeintake/tags wiring built with pulumi.Sprintf) must be supplied as plain config
// strings by the environment-level installer instead.
func resolveExtraAgentConfig(extra []pulumi.StringInput) ([]string, error) {
	out := make([]string, 0, len(extra))
	for _, in := range extra {
		s, ok := in.(pulumi.String)
		if !ok {
			return nil, fmt.Errorf("the SSH agent installer cannot resolve a computed pulumi value in ExtraAgentConfig; " +
				"supply such config (e.g. fakeintake, tags, hostname) as a plain string via the environment-level installer")
		}
		out = append(out, string(s))
	}
	return out, nil
}

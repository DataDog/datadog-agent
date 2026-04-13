// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	oscomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

// SetAgentConfig reconfigures the agent on a provisioned host without
// re-running Pulumi. It writes config files via SSH and restarts the agent.
//
// This accepts the same agentparams options used with provisioners:
//
//	SetAgentConfig(s.T(), s.Env().RemoteHost, s.Env().Agent.Client,
//	    agentparams.WithAgentConfig("log_level: info"),
//	)
//
// The function writes the relevant config files (datadog.yaml,
// system-probe.yaml, security-agent.yaml), integration configs, and arbitrary
// files to the host, then restarts the agent and waits for it to be ready.
func SetAgentConfig(t *testing.T, host *components.RemoteHost, agent agentclient.Agent, opts ...agentparams.Option) {
	t.Helper()

	p := &agentparams.Params{
		Integrations: make(map[string]*agentparams.FileDefinition),
		Files:        make(map[string]*agentparams.FileDefinition),
	}
	for _, opt := range opts {
		require.NoError(t, opt(p))
	}

	configFolder, err := host.GetAgentConfigFolder()
	require.NoError(t, err, "failed to get agent config folder")

	// Write datadog.yaml
	if p.AgentConfig != "" {
		writeConfigFile(t, host, host.JoinPath(configFolder, "datadog.yaml"), p.AgentConfig)
	}

	// Write system-probe.yaml
	if p.SystemProbeConfig != "" {
		writeConfigFile(t, host, host.JoinPath(configFolder, "system-probe.yaml"), p.SystemProbeConfig)
	}

	// Write security-agent.yaml
	if p.SecurityAgentConfig != "" {
		writeConfigFile(t, host, host.JoinPath(configFolder, "security-agent.yaml"), p.SecurityAgentConfig)
	}

	// Write integration configs (conf.d/<name>/conf.yaml)
	for confPath, fileDef := range p.Integrations {
		fullPath := host.JoinPath(configFolder, confPath)
		mkdirPrivileged(t, host, dirPath(host, fullPath))
		writeFileDefinition(t, host, fullPath, fileDef)
	}

	// Write arbitrary files
	for filePath, fileDef := range p.Files {
		dir := dirPath(host, filePath)
		if fileDef.UseSudo {
			mkdirPrivileged(t, host, dir)
		} else {
			require.NoError(t, host.MkdirAll(dir), "failed to create directory %s", dir)
		}
		writeFileDefinition(t, host, filePath, fileDef)
	}

	// Restart the agent and wait for ready.
	//
	// On Linux with systemd, sub-agent services (system-probe, security-agent)
	// use BindsTo=datadog-agent.service, so restarting the main agent
	// automatically restarts them. The ConditionPathExists directive ensures
	// they only start when their config file exists.
	//
	// For non-systemd Linux or if BindsTo is not configured, we explicitly
	// restart sub-agents after the main agent.
	require.NoError(t, agent.Restart(), "failed to restart agent")
	restartSubAgentServices(t, host, p)
	require.Eventually(t, agent.IsReady, 2*time.Minute, 5*time.Second, "agent not ready after config change")
}

// restartSubAgentServices restarts sub-agent services whose configs changed.
//
// On Linux, system-probe and security-agent are separate services;
// restarting datadog-agent does NOT restart them. The init system is
// auto-detected (systemctl, initctl, or service command).
//
// On Windows, all agents run under a single service managed by agent.exe,
// so no extra restarts are needed.
//
// On macOS, sub-agents are managed by the main launchd service.
func restartSubAgentServices(t *testing.T, host *components.RemoteHost, p *agentparams.Params) {
	t.Helper()

	switch host.OSFamily {
	case oscomp.LinuxFamily:
		services := make([]string, 0, 2)
		if p.SystemProbeConfig != "" {
			services = append(services, "datadog-agent-sysprobe")
		}
		if p.SecurityAgentConfig != "" {
			services = append(services, "datadog-agent-security")
		}
		for _, svc := range services {
			restartLinuxService(host, svc)
		}
	case oscomp.WindowsFamily:
		// On Windows, agent.exe restart-service handles all sub-agents.
	case oscomp.MacOSFamily:
		// On macOS, the main launchd service manages sub-agents.
	}
}

// restartLinuxService restarts a service on Linux, auto-detecting the init
// system. Errors are ignored since the service may not be installed.
func restartLinuxService(host *components.RemoteHost, service string) {
	if _, err := host.Execute("command -v systemctl"); err == nil {
		_, _ = host.Execute(fmt.Sprintf("sudo systemctl restart %s", service))
		return
	}
	if _, err := host.Execute("command -v /sbin/initctl"); err == nil {
		// upstart: stop then start, since restart fails if not running
		_, _ = host.Execute(fmt.Sprintf("sudo /sbin/initctl stop %s; sudo /sbin/initctl start %s", service, service))
		return
	}
	// Fallback to generic service command
	_, _ = host.Execute(fmt.Sprintf("sudo service %s restart", service))
}

// writeConfigFile writes a config file to the host using elevated privileges.
// Mirrors the approach used by the Pulumi agent component's CopyInlineFile.
func writeConfigFile(t *testing.T, host *components.RemoteHost, fullPath string, content string) {
	t.Helper()
	switch host.OSFamily {
	case oscomp.WindowsFamily:
		// On Windows, write via PowerShell here-string + Set-Content.
		host.MustExecute(fmt.Sprintf("Set-Content -Path '%s' -Value @\"\n%s\n\"@", fullPath, content))
	default:
		// On Linux/macOS, use sudo tee (agent config files are owned by dd-agent).
		host.MustExecute(fmt.Sprintf("sudo tee %s > /dev/null << 'AGENTCONFIGEOF'\n%s\nAGENTCONFIGEOF", fullPath, content))
	}
}

// writeFileDefinition writes a file and applies permissions from a FileDefinition.
func writeFileDefinition(t *testing.T, host *components.RemoteHost, fullPath string, fileDef *agentparams.FileDefinition) {
	t.Helper()

	if fileDef.UseSudo {
		writeConfigFile(t, host, fullPath, fileDef.Content)
	} else {
		_, err := host.WriteFile(fullPath, []byte(fileDef.Content))
		require.NoError(t, err, "failed to write file %s", fullPath)
	}

	// Apply permissions if specified (permissions commands are already OS-aware)
	if perms, ok := fileDef.Permissions.Get(); ok {
		cmd := perms.SetupPermissionsCommand(fullPath)
		if cmd != "" {
			host.MustExecute(cmd)
		}
	}
}

// mkdirPrivileged creates a directory with elevated privileges.
func mkdirPrivileged(t *testing.T, host *components.RemoteHost, dir string) {
	t.Helper()
	switch host.OSFamily {
	case oscomp.WindowsFamily:
		host.MustExecute(fmt.Sprintf("New-Item -ItemType Directory -Force -Path '%s'", dir))
	default:
		host.MustExecute(fmt.Sprintf("sudo mkdir -p %s", dir))
	}
}

// dirPath returns the parent directory of a file path.
func dirPath(host *components.RemoteHost, filePath string) string {
	switch host.OSFamily {
	case oscomp.WindowsFamily:
		for i := len(filePath) - 1; i >= 0; i-- {
			if filePath[i] == '\\' || filePath[i] == '/' {
				return filePath[:i]
			}
		}
	default:
		for i := len(filePath) - 1; i >= 0; i-- {
			if filePath[i] == '/' {
				return filePath[:i]
			}
		}
	}
	return filePath
}

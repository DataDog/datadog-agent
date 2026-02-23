// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package assertions

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	e2ecommon "github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/stretchr/testify/require"
)

const (
	defaultAgentBinPath = "C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe"
)

// RemoteWindowsHostAssertions is a type that extends the SuiteAssertions to add assertions
// executing on a RemoteHost.
type RemoteWindowsHostAssertions struct {
	// Don't embed the "require.Assertions" type because that could confuse the caller as to which code executes
	// on the remoteHost vs locally.
	// With a "private" require.Assertions, when the caller uses suite.Require().Host() they will
	// only get access to the assertions that can effectively run on the remote server, preventing
	// accidental misuse.
	require    *require.Assertions
	context    e2ecommon.Context
	remoteHost *components.RemoteHost
}

// New returns a new RemoteWindowsHostAssertions
func New(context e2ecommon.Context, assertions *require.Assertions, remoteHost *components.RemoteHost) *RemoteWindowsHostAssertions {
	return &RemoteWindowsHostAssertions{
		require:    assertions,
		context:    context,
		remoteHost: remoteHost,
	}
}

// HasAService returns an assertion object that can be used to assert things about
// a given Windows service. If the service doesn't exist, it fails.
func (r *RemoteWindowsHostAssertions) HasAService(serviceName string) *RemoteWindowsServiceAssertions {
	r.context.T().Helper()
	serviceConfig, err := common.GetServiceConfig(r.remoteHost, serviceName)
	r.require.NoError(err)
	return &RemoteWindowsServiceAssertions{RemoteWindowsHostAssertions: r, serviceConfig: serviceConfig}
}

// HasNoService returns an assertion object that can be used to assert things about
// a given Windows service. If the service doesn't exist, it fails.
func (r *RemoteWindowsHostAssertions) HasNoService(serviceName string) *RemoteWindowsHostAssertions {
	r.context.T().Helper()
	_, err := common.GetServiceConfig(r.remoteHost, serviceName)
	r.require.Error(err)
	return r
}

// DirExists checks whether a directory exists in the given path. It also fails if
// the path points to a directory or there is an error when trying to check the file.
func (r *RemoteWindowsHostAssertions) DirExists(path string, msgAndArgs ...interface{}) *RemoteWindowsHostAssertions {
	r.context.T().Helper()
	_, err := r.remoteHost.Lstat(path)
	r.require.NoError(err, msgAndArgs...)
	return r
}

// NoDirExists checks whether a directory does not exist in the given path.
func (r *RemoteWindowsHostAssertions) NoDirExists(path string, msgAndArgs ...interface{}) *RemoteWindowsHostAssertions {
	r.context.T().Helper()
	_, err := r.remoteHost.Lstat(path)
	r.require.ErrorIs(err, fs.ErrNotExist, msgAndArgs...)
	return r
}

// FileExists checks whether a file exists in the given path. It also fails if
// the path points to a directory or there is an error when trying to check the file.
func (r *RemoteWindowsHostAssertions) FileExists(path string, msgAndArgs ...interface{}) *RemoteWindowsHostAssertions {
	r.context.T().Helper()
	exists, err := r.remoteHost.FileExists(path)
	r.require.NoError(err)
	r.require.True(exists, msgAndArgs...)
	return r
}

// NoFileExists checks whether a file does not exist in the given path. It also fails if
// the path points to a directory or there is an error when trying to check the file.
func (r *RemoteWindowsHostAssertions) NoFileExists(path string, msgAndArgs ...interface{}) *RemoteWindowsHostAssertions {
	r.context.T().Helper()
	exists, err := r.remoteHost.FileExists(path)
	r.require.NoError(err)
	r.require.False(exists, msgAndArgs...)
	return r
}

// HasARunningDatadogAgentService checks if the remote host has a Datadog Agent installed & running.
// It does not run a full test suite on it, but merely checks if it has the required
// service running.
func (r *RemoteWindowsHostAssertions) HasARunningDatadogAgentService() *RemoteWindowsAgentAssertions {
	r.context.T().Helper()

	installPath, err := windowsagent.GetInstallPathFromRegistry(r.remoteHost)
	r.require.NoError(err)
	binPath := installPath + `\bin\agent.exe`
	r.FileExists(binPath)

	r.HasAService("datadogagent").WithStatus("Running")

	configRoot, err := windowsagent.GetConfigRootFromRegistry(r.remoteHost)
	r.require.NoError(err)
	ipcCertPath := filepath.Join(configRoot, "ipc_cert.pem")

	agentClient, err := client.NewHostAgentClientWithParams(r.context, r.remoteHost.HostOutput,
		agentclientparams.WithAgentInstallPath(installPath),
		agentclientparams.WithIPCCertPath(ipcCertPath),
		agentclientparams.WithSkipWaitForAgentReady(),
	)
	r.require.NoError(err)

	return &RemoteWindowsAgentAssertions{
		RemoteWindowsBinaryAssertions: &RemoteWindowsBinaryAssertions{
			RemoteWindowsHostAssertions: r,
			binaryPath:                  binPath,
		},
		agentClient: agentClient,
	}
}

// HasNoDatadogAgentService checks if the remote host doesn't have a Datadog Agent installed.
func (r *RemoteWindowsHostAssertions) HasNoDatadogAgentService() *RemoteWindowsBinaryAssertions {
	r.context.T().Helper()
	r.NoFileExists(defaultAgentBinPath)
	r.HasNoService("datadogagent")
	return &RemoteWindowsBinaryAssertions{
		RemoteWindowsHostAssertions: r,
		binaryPath:                  defaultAgentBinPath,
	}
}

// HasBinary checks if a binary exists on the remote host and returns a more specific assertion
// allowing to run further tests on the binary.
func (r *RemoteWindowsHostAssertions) HasBinary(path string) *RemoteWindowsBinaryAssertions {
	r.context.T().Helper()
	r.FileExists(path)
	return &RemoteWindowsBinaryAssertions{
		RemoteWindowsHostAssertions: r,
		binaryPath:                  path,
	}
}

// HasRegistryKey checks if a registry key exists on the remote host.
func (r *RemoteWindowsHostAssertions) HasRegistryKey(key string) *RemoteWindowsRegistryKeyAssertions {
	r.context.T().Helper()
	exists, err := common.RegistryKeyExists(r.remoteHost, key)
	r.require.NoError(err)
	r.require.True(exists)
	return &RemoteWindowsRegistryKeyAssertions{
		RemoteWindowsHostAssertions: r,
		keyPath:                     key,
	}
}

// HasNoRegistryKey checks if a registry key does not exist on the remote host.
func (r *RemoteWindowsHostAssertions) HasNoRegistryKey(key string) *RemoteWindowsHostAssertions {
	r.context.T().Helper()
	exists, err := common.RegistryKeyExists(r.remoteHost, key)
	r.require.NoError(err)
	r.require.False(exists)
	return r
}

// HasNamedPipe checks if a named pipe exists on the remote host
func (r *RemoteWindowsHostAssertions) HasNamedPipe(pipeName string) *RemoteWindowsNamedPipeAssertions {
	r.context.T().Helper()

	cmd := fmt.Sprintf("Test-Path '%s'", pipeName)
	out, err := r.remoteHost.Execute(cmd)
	r.require.NoError(err)
	out = strings.TrimSpace(out)
	r.require.Equal("True", out)

	return &RemoteWindowsNamedPipeAssertions{
		RemoteWindowsHostAssertions: r,
		pipename:                    pipeName,
	}
}

// HasNoNamedPipe checks if a named pipe does not exist on the remote host
func (r *RemoteWindowsHostAssertions) HasNoNamedPipe(pipeName string) *RemoteWindowsHostAssertions {
	r.context.T().Helper()

	cmd := fmt.Sprintf("Test-Path '%s'", pipeName)
	out, err := r.remoteHost.Execute(cmd)
	r.require.NoError(err)
	out = strings.TrimSpace(out)
	r.require.Equal("False", out)

	return r
}

// HasARunningDatadogInstallerService verifies that the Datadog Installer service is installed and correctly configured.
func (r *RemoteWindowsHostAssertions) HasARunningDatadogInstallerService() *RemoteWindowsHostAssertions {
	r.context.T().Helper()

	r.HasAService(consts.ServiceName).
		WithStatus("Running").
		HasNamedPipe(consts.NamedPipe).
		WithSecurity(
			// Only accessible to Administrators and LocalSystem
			common.NewProtectedSecurityInfo(
				common.GetIdentityForSID(common.AdministratorsSID),
				common.GetIdentityForSID(common.LocalSystemSID),
				[]common.AccessRule{
					common.NewExplicitAccessRule(
						common.GetIdentityForSID(common.LocalSystemSID),
						common.FileFullControl,
						common.AccessControlTypeAllow,
					),
					common.NewExplicitAccessRule(
						common.GetIdentityForSID(common.AdministratorsSID),
						common.FileFullControl,
						common.AccessControlTypeAllow,
					),
				},
			))
	return r
}

// HasDatadogInstaller verifies that the Datadog Installer is installed on the remote host.
func (r *RemoteWindowsHostAssertions) HasDatadogInstaller() *RemoteWindowsInstallerAssertions {
	r.context.T().Helper()

	installPath, err := windowsagent.GetInstallPathFromRegistry(r.remoteHost)
	r.require.NoError(err)
	bin := r.HasBinary(installPath + `\bin\` + consts.BinaryName)
	return &RemoteWindowsInstallerAssertions{
		RemoteWindowsBinaryAssertions: bin,
	}
}

// HasDDAgentUserFileAccess verifies that ddagentuser has appropriate permissions
// on key Agent files and directories. This is to verify that config updates
// or upgrades haven't broken file permissions.
func (r *RemoteWindowsHostAssertions) HasDDAgentUserFileAccess(args ...string) *RemoteWindowsHostAssertions {
	r.context.T().Helper()

	var agentUserName string
	if len(args) > 0 {
		agentUserName = args[0]
	} else {
		agentUserName = windowsagent.DefaultAgentUserName
	}

	// Get ddagentuser identity
	ddAgentUser, err := common.GetIdentityForUser(r.remoteHost, agentUserName)
	r.require.NoError(err, "should get ddagentuser identity")

	// Get config root from registry
	configRoot, err := windowsagent.GetConfigRootFromRegistry(r.remoteHost)
	r.require.NoError(err, "should get config root from registry")

	// Test critical paths that ddagentuser needs access to
	criticalPaths := []struct {
		path        string
		minRights   int
		description string
		fileType    string
	}{
		{
			path:        filepath.Join(configRoot, "datadog.yaml"),
			minRights:   common.FileFullControl,
			description: "datadog.yaml must be readable by ddagentuser",
			fileType:    "file",
		},
		{
			path:        filepath.Join(configRoot, "conf.d"),
			minRights:   common.FileFullControl,
			description: "conf.d must be readable by ddagentuser",
			fileType:    "directory",
		},
		{
			path:        filepath.Join(configRoot, "logs"),
			minRights:   common.FileFullControl,
			description: "logs directory must be writable by ddagentuser",
			fileType:    "directory",
		},
	}

	for _, cp := range criticalPaths {
		switch cp.fileType {
		case "file":
			exists, err := r.remoteHost.FileExists(cp.path)
			r.require.NoError(err, "should check if file exists")
			if !exists {
				r.require.Failf("path %s does not exist", cp.path)
			}
		case "directory":
			_, err := r.remoteHost.Lstat(cp.path)
			r.require.NoError(err, "should check if directory exists")
		}

		security, err := common.GetSecurityInfoForPath(r.remoteHost, cp.path)
		r.require.NoError(err, "should get security info for %s", cp.path)

		// Filter to get ddagentuser rules
		ddagentRules := common.FilterRulesForIdentity(security.Access, ddAgentUser)
		r.require.NotEmpty(ddagentRules,
			"ddagentuser should have access rules on %s", cp.path)

		// Verify at least one rule grants the minimum required rights
		hasRequiredRights := false
		for _, rule := range ddagentRules {
			if rule.IsAllow() && (rule.Rights&cp.minRights) == cp.minRights {
				hasRequiredRights = true
				break
			}
		}

		r.require.True(hasRequiredRights,
			"%s (path: %s)", cp.description, cp.path)
	}

	return r
}

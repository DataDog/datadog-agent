// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !e2eunit

package installer

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

type testInstallExeUntrustedConfigRootSuite struct {
	BaseSuite
}

func TestInstallExeWithUntrustedConfigRoot(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &testInstallExeUntrustedConfigRootSuite{},
		e2e.WithProvisioner(winawshost.ProvisionerNoAgentNoFakeIntake()))
}

// TestUntrustedConfigEmitsNoTelemetry exercises the elevated executable's default setup path,
// with a planted destination in a directory owned by the untrusted BUILTIN\Users group.
func (s *testInstallExeUntrustedConfigRootSuite) TestUntrustedConfigEmitsNoTelemetry() {
	vm := s.Env().RemoteHost
	receiver := windowsCommon.StartConfigTrustReceiver(s.T(), vm, s.SessionOutputDir())
	exe := NewDatadogInstallExe(vm, WithInstallScriptDevEnvOverrides("CURRENT_AGENT"))
	url, err := exe.getInstallerURL(exe.params)
	s.Require().NoError(err)
	binary, err := windowsCommon.GetTemporaryFile(vm)
	s.Require().NoError(err)
	binary += ".exe"
	s.Require().NoError(windowsCommon.PutOrDownloadFile(vm, url, binary))
	s.T().Cleanup(func() { s.Assert().NoError(vm.Remove(binary)) })

	// A real command must emit a trace to the same receiver with an authorized destination.
	// Force retention because get-states is sampled; the setup helper cannot pass command arguments.
	output, err := vm.Execute(fmt.Sprintf(`& '%s' get-states; exit $LASTEXITCODE`, binary),
		client.WithEnvVariables(map[string]string{
			"DD_API_KEY":                        windowsCommon.ConfigTrustAPIKey,
			"DD_SITE":                           windowsCommon.ConfigTrustSite,
			"DD_APPLICATIONDATADIRECTORY":       "",
			"DD_INSTALLER_FROM_VERSION_HANDOFF": "true",
			"DATADOG_SAMPLING_PRIORITY":         "2",
		}))
	s.Require().NoErrorf(err, "telemetry positive-control command failed: %s", output)
	receiver.RequireTelemetry(s.T(), "")
	receiver.ClearConfigRoot(s.T())
	receiver.Reset(s.T())
	receiver.PlantConfigRoot(s.T())

	// Remove the helper's default destination so only the planted YAML can supply it.
	// Run waits for process exit, including the shutdown telemetry flush.
	_, err = exe.Run(WithExtraEnvVars(map[string]string{
		"DD_API_KEY":                  windowsCommon.ConfigTrustAPIKey,
		"DD_SITE":                     "",
		"DD_APPLICATIONDATADIRECTORY": "",
	}))
	// RemoteHost.Execute includes the command output in err on a nonzero exit.
	s.Require().ErrorContains(err, "has unexpected owner", "elevated setup must reject the untrusted configuration directory")
	receiver.AssertNoSubmissions(s.T())
	config, err := vm.ReadFile(windowsAgent.DefaultConfigRoot + `\datadog.yaml`)
	s.Require().NoError(err)
	s.Require().Equal("site: "+windowsCommon.ConfigTrustSite+"\n", string(config))
	for _, service := range []string{"datadogagent", "Datadog Installer"} {
		_, err := windowsCommon.GetServiceConfig(vm, service)
		s.Require().Errorf(err, "rejection must not install or start %s", service)
	}
}

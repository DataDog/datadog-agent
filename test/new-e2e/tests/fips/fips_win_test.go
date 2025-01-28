// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package fips

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsHostWindows "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/components/fipsmode"

	"testing"

	"github.com/stretchr/testify/assert"
)

//go:embed fixtures/e2e_fips_test.py
var fipsTestCheck string

type windowsVMSuite struct {
	e2e.BaseSuite[environments.WindowsHost]

	installPath string
}

func TestWindowsVM(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(awsHostWindows.Provisioner(
		// Enable FIPS mode on the host (done before Agent install)
		awsHostWindows.WithFIPSModeOptions(fipsmode.WithFIPSModeEnabled()),
		// Use FIPS Agent package
		awsHostWindows.WithAgentOptions(
			agentparams.WithFlavor(agentparams.FIPSFlavor),
			agentparams.WithFile(
				`C:/ProgramData/Datadog/checks.d/e2e_fips_test.py`,
				fipsTestCheck,
				false,
			),
			agentparams.WithFile(
				`C:/ProgramData/Datadog/conf.d/e2e_fips_test.yaml`,
				`
init_config:
instances: [{}]
`,
				false,
			),
		),
	))}

	e2e.Run(t, &windowsVMSuite{}, suiteParams...)
}

func (s *windowsVMSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	host := s.Env().RemoteHost
	var err error

	s.installPath, err = windowsAgent.GetInstallPathFromRegistry(host)
	s.Require().NoError(err)
}

func (s *windowsVMSuite) TestVersionCommands() {
	host := s.Env().RemoteHost
	windowsCommon.EnableFIPSMode(host)
	s.Run("System FIPS Enabled", func() {
		s.testAgentBinaries(func(executable string) {
			var err error
			_, err = s.execAgentCommandWithFIPS(executable, "version")
			s.Assert().NoError(err)
			_, err = s.execAgentCommand(executable, "version")
			s.Assert().NoError(err)
		})
	})
	windowsCommon.DisableFIPSMode(host)
	s.Run("System FIPS Disabled", func() {
		s.testAgentBinaries(func(executable string) {
			var err error
			_, err = s.execAgentCommandWithFIPS(executable, "version")
			assertErrorContainsFIPSPanic(s.T(), err, "agent should panic when GOFIPS=1 but system FIPS is disabled")
			_, err = s.execAgentCommand(executable, "version")
			s.Assert().NoError(err)
		})
	})
}

// TestReportsFIPSStatusMetrics tests that the custom check from our fixtures
// is able to report metrics while in FIPS mode. These metric values are based
// on the status of Python's FIPS mode.
func (s *windowsVMSuite) TestReportsFIPSStatusMetrics() {
	// Install custom check
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("e2e.fips_mode", fakeintakeclient.WithMetricValueHigherThan(0))
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'e2e.fips_mode' with value higher than 0 yet")

		metrics, err = s.Env().FakeIntake.Client().FilterMetrics("e2e.fips_dll_loaded", fakeintakeclient.WithMetricValueHigherThan(0))
		assert.NoError(c, err)
		assert.Greater(c, len(metrics), 0, "no 'e2e.fips_dll_loaded' with value higher than 0 yet")
	}, 5*time.Minute, 10*time.Second)
}

func (s *windowsVMSuite) testAgentBinaries(subtest func(executable string)) {
	executables := []string{"agent.exe", "agent/system-probe.exe", "agent/trace-agent.exe",
		"agent/process-agent.exe", "agent/security-agent.exe"}
	for _, executable := range executables {
		s.Run(executable, func() {
			subtest(executable)
		})
	}
}

func (s *windowsVMSuite) execAgentCommand(executable, command string, options ...client.ExecuteOption) (string, error) {
	host := s.Env().RemoteHost

	s.Require().NotEmpty(s.installPath)
	agentPath := filepath.Join(s.installPath, "bin", executable)

	cmd := fmt.Sprintf(`& "%s" %s`, agentPath, command)
	return host.Execute(cmd, options...)
}

func (s *windowsVMSuite) execAgentCommandWithFIPS(executable, command string) (string, error) {
	// There isn't support for appending env vars to client.ExecuteOption, so
	// this function doesn't accept any other options.

	// Setting GOFIPS=1 causes the Windows FIPS Agent to panic if the system is not in FIPS mode.
	// This setting does NOT control whether the FIPS Agent uses FIPS-compliant crypto libraries,
	// the System-level setting determines that.
	// https://github.com/microsoft/go/tree/microsoft/main/eng/doc/fips#windows-fips-mode-cng
	vars := client.EnvVar{
		"GOFIPS": "1",
	}

	return s.execAgentCommand(executable, command, client.WithEnvVariables(vars))
}

func assertErrorContainsFIPSPanic(t *testing.T, err error, args ...interface{}) bool {
	return assert.ErrorContains(t, err, "panic: cngcrypto: not in FIPS mode", args...)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package windowsstartup contains e2e tests that check the Agent Data Plane (ADP) starts and stays up on each
// supported Windows Server release.
//
// ADP runs TLS through the platform crypto provider, which on Windows is CNG. CNG's feature set differs between
// Windows releases, so a build that is healthy on the default Windows Server image can still crash on an older one.
// Windows Server 2016 is the oldest supported release and the one most likely to lack a CNG algorithm, so it gets
// its own suite.
package windowsstartup

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	// adpProcessName is the dd-procmgr name of ADP, from the datadog-agent-data-plane.yaml processes.d file.
	adpProcessName = "datadog-agent-data-plane"

	// procmgrServiceName is the Windows service that runs dd-procmgrd. It is DEMAND_START.
	procmgrServiceName = "dd-procmgr-service"

	// startTimeout is how long ADP has to reach Running after dd-procmgr starts.
	startTimeout = 5 * time.Minute

	// holdFor is how long ADP must stay Running as one process. ADP connects to the Core Agent over TLS 1.3 IPC
	// within seconds of starting, so a TLS failure shows well inside this window. It is also longer than the
	// roughly 60 second restart loop reported on Windows Server 2016, so the loop cannot hide between two polls.
	holdFor = 3 * time.Minute

	// holdTimeout is the deadline to reach a complete hold, not the length of it.
	holdTimeout = holdFor + 2*time.Minute

	pollInterval = 5 * time.Second
)

// adpWindowsStartupSuite checks that ADP, enabled in datadog.yaml, reaches Running under dd-procmgr and then stays
// Running as one process with no restarts.
type adpWindowsStartupSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestADPStartupWindowsServer2016Suite runs the check on Windows Server 2016.
//
// CNG on Windows Server 2016 has no native HKDF. The CNG rustls provider uses native HKDF in every TLS 1.3
// handshake and panics when it is missing, so ADP crashes on its first IPC connection to the Core Agent. This suite
// fails until the provider has a fallback.
func TestADPStartupWindowsServer2016Suite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &adpWindowsStartupSuite{}, suiteOptions(e2eos.WindowsServer2016)...)
}

// TestADPStartupWindowsServerDefaultSuite runs the same check on the default Windows Server image.
//
// It is the control for the Windows Server 2016 suite: when only the 2016 suite fails, the failure comes from the
// OS release and not from the test or the build.
func TestADPStartupWindowsServerDefaultSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &adpWindowsStartupSuite{}, suiteOptions(e2eos.WindowsServerDefault)...)
}

func suiteOptions(descriptor e2eos.Descriptor) []e2e.SuiteOption {
	return []e2e.SuiteOption{
		e2e.WithProvisioner(
			awshost.ProvisionerNoFakeIntake(
				awshost.WithRunOptions(
					ec2.WithEC2InstanceOptions(ec2.WithOS(descriptor), ec2.WithInternetAccess()),
					ec2.WithAgentOptions(agentparams.WithAgentConfig("data_plane.enabled: true")),
				),
			),
		),
	}
}

func (s *adpWindowsStartupSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost

	// The test must not skip when dd-procmgr is missing: a skip reports as a pass, and this suite exists to fail.
	exists, err := host.FileExists(s.procmgrCLI())
	require.NoError(s.T(), err)
	require.True(s.T(), exists, "dd-procmgr CLI should be installed at %s", s.procmgrCLI())

	require.NoError(s.T(), windowsCommon.StartService(host, procmgrServiceName))
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(fmt.Sprintf(`(Get-Service '%s').Status`, procmgrServiceName))
		if !assert.NoError(ct, err) {
			return
		}
		assert.Equal(ct, "Running", strings.TrimSpace(out))
	}, time.Minute, 2*time.Second)
}

// TestADPStaysUp requires ADP to reach Running and then to hold one PID, with an unchanged restart count, for
// holdFor.
func (s *adpWindowsStartupSuite) TestADPStaysUp() {
	host := s.Env().RemoteHost
	defer func() {
		if s.T().Failed() {
			s.dumpDiagnostics()
		}
	}()

	var (
		pid, restarts string
		heldSince     time.Time
	)
	reset := func() {
		pid, restarts, heldSince = "", "", time.Time{}
	}

	// Phase 1: ADP reaches Running.
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(s.describeADP())
		if !assert.NoError(ct, err, "dd-procmgr describe %s", adpProcessName) {
			return
		}
		assert.Equal(ct, "Running", fieldValue(out, "State"), "%s should be Running: %s", adpProcessName, out)
	}, startTimeout, pollInterval, "%s never reached Running", adpProcessName)

	// Phase 2: ADP stays Running as the same process with no new restarts. Any change resets the hold. A crash
	// loop changes the PID or the restart count, or shows a state other than Running, at least once per restart.
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out, err := host.Execute(s.describeADP())
		if !assert.NoError(ct, err, "dd-procmgr describe %s", adpProcessName) {
			reset()
			return
		}

		state, p, r := fieldValue(out, "State"), fieldValue(out, "PID"), fieldValue(out, "Restarts")
		if !assert.Equal(ct, "Running", state, "%s should stay Running: %s", adpProcessName, out) ||
			!assert.NotContains(ct, []string{"", "-"}, p, "a Running process should have a PID: %s", out) {
			reset()
			return
		}

		now := time.Now()
		if p != pid || r != restarts {
			if pid != "" {
				s.T().Logf("%s changed from PID %s (restarts %s) to PID %s (restarts %s)",
					adpProcessName, pid, restarts, p, r)
			}
			pid, restarts, heldSince = p, r, now
		}

		// A tick that records no failure ends the wait, so the hold has to be an assertion.
		assert.GreaterOrEqual(ct, now.Sub(heldSince), holdFor,
			"%s has held PID %s for %s, needs %s", adpProcessName, pid, now.Sub(heldSince).Round(time.Second), holdFor)
	}, holdTimeout, pollInterval, "%s did not stay up as one process for %s", adpProcessName, holdFor)
}

// dumpDiagnostics logs what is needed to tell a crash loop from other failures. The host is destroyed when the
// suite ends, so a CI failure cannot be investigated afterwards. Best-effort: each step logs its own error.
func (s *adpWindowsStartupSuite) dumpDiagnostics() {
	host := s.Env().RemoteHost

	s.logCommand("dd-procmgr describe", s.describeADP())
	s.logCommand("OS version", `(Get-CimInstance Win32_OperatingSystem | Select-Object Caption, Version | Format-List | Out-String).Trim()`)

	configRoot, err := windowsagent.GetConfigRootFromRegistry(host)
	if err != nil {
		s.T().Logf("could not read ConfigRoot from the registry: %v", err)
		return
	}
	logDir := strings.TrimRight(configRoot, `\`) + `\logs`

	// The panic message goes to stderr, not to the ADP log, so also print the dd-procmgr logs that capture
	// child output.
	s.logCommand("log files", fmt.Sprintf(`Get-ChildItem -LiteralPath '%s' -Filter *.log | Format-Table Name, Length, LastWriteTime | Out-String`, logDir))
	s.logCommand("panic lines", fmt.Sprintf(`Select-String -Path '%s\*.log' -Pattern 'panicked' | Select-Object -Last 20 | Out-String`, logDir))
	s.logCommand("agent-data-plane.log", fmt.Sprintf(`Get-Content -LiteralPath '%s\agent-data-plane.log' -Tail 50 -ErrorAction SilentlyContinue | Out-String`, logDir))
	s.logCommand("procmgr logs", fmt.Sprintf(`Get-ChildItem -LiteralPath '%s' -Filter '*procmgr*.log' | ForEach-Object { "== $($_.Name)"; Get-Content -LiteralPath $_.FullName -Tail 50 } | Out-String`, logDir))
}

func (s *adpWindowsStartupSuite) logCommand(label, command string) {
	out, err := s.Env().RemoteHost.Execute(command)
	if err != nil {
		s.T().Logf("%s: %v", label, err)
		return
	}
	s.T().Logf("%s:\n%s", label, strings.TrimSpace(out))
}

func (s *adpWindowsStartupSuite) procmgrCLI() string {
	return s.agentBin("dd-procmgr.exe")
}

func (s *adpWindowsStartupSuite) describeADP() string {
	return fmt.Sprintf(`& '%s' describe %s`, s.procmgrCLI(), adpProcessName)
}

func (s *adpWindowsStartupSuite) agentBin(name string) string {
	installPath, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	require.NoError(s.T(), err)
	return strings.TrimRight(installPath, `\`) + `\bin\agent\` + name
}

// fieldValue returns the value of a "Label: value" line in dd-procmgr describe output.
func fieldValue(output, label string) string {
	needle := label + ":"
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, needle) {
			return strings.TrimSpace(trimmed[len(needle):])
		}
	}
	return ""
}

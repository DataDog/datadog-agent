// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	awsfakeintake "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// parExecutorPipe is the executor's named pipe in the Windows process definition.
const parExecutorPipe = `\\.\pipe\dd-par-executor`

// Access masks the executor grants on its pipe (see
// pkg/privateactionrunner/executor/transport_windows.go).
const (
	pipeFullAccess   = 0x1f01ff // FILE_ALL_ACCESS, Local System and Administrators
	pipeServerAccess = 0x12019f // FILE_GENERIC_READ | FILE_GENERIC_WRITE, the pipe creator
	pipeClientAccess = 0x12008b // FILE_GENERIC_READ | FILE_WRITE_DATA, the Agent account
)

// Win32 errors returned by the probe.
const (
	errorAccessDenied = 5 // ERROR_ACCESS_DENIED
	errorFileNotFound = 2 // ERROR_FILE_NOT_FOUND: the executor idled out mid-probe
)

//go:embed fixtures/windows-pipe-probe.ps1
var pipeProbeScript []byte

// pipeProbe is the JSON printed by fixtures/windows-pipe-probe.ps1.
type pipeProbe struct {
	SDDL      string `json:"sddl"`
	Owner     string `json:"owner"`
	Protected bool   `json:"protected"`
	ACEs      []struct {
		SID  string `json:"sid"`
		Mask int64  `json:"mask"`
		Type string `json:"type"`
	} `json:"aces"`
	ProbeError   int    `json:"probeError"`
	AgentSID     string `json:"agentSid"`
	AgentIsAdmin bool   `json:"agentIsAdmin"`
	ExecutorSID  string `json:"executorSid"`
}

// parMonolithProcess is the always-installed monolithic runner definition. It must
// stand down while split mode is active.
const parMonolithProcess = "datadog-agent-action"

type windowsPARSplitLifecycleSuite struct {
	e2e.BaseSuite[environments.Host]

	installRoot string
	signingKey  testSigningKey
}

func TestWindowsPARSplitLifecycleSuite(t *testing.T) {
	t.Parallel()
	urn, privateKey := GenerateTestRunnerIdentity(t)
	config := fmt.Sprintf(`private_action_runner:
  enabled: true
  split_enabled: true
  self_enroll: false
  urn: %s
  private_key: %s
  idle_timeout_seconds: 5
  actions_allowlist:
    - %s
`, urn, privateKey, runCommandAction)

	suite := &windowsPARSplitLifecycleSuite{signingKey: generateTestSigningKey(t, "windows-runner-key")}
	e2e.Run(t, suite, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithEC2InstanceOptions(scenec2.WithOS(e2eos.WindowsServerDefault), scenec2.WithInternetAccess()),
				scenec2.WithFakeIntakeOptions(awsfakeintake.WithLoadBalancer()),
				scenec2.WithAgentOptions(agentparams.WithAgentConfig(config)),
			),
		),
	))
}

func (s *windowsPARSplitLifecycleSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// Everything below can fail, so the framework needs the failure hook.
	defer s.CleanupOnSetupFailure()

	installRoot, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	s.Require().NoError(err)
	s.installRoot = installRoot

	exists, err := s.Env().RemoteHost.FileExists(s.parControlBinary())
	s.Require().NoError(err)
	s.Require().True(exists, "par-control.exe should be installed at %s", s.parControlBinary())

	_, err = s.Env().RemoteHost.Execute(`[Environment]::SetEnvironmentVariable('DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS', 'true', 'Machine')`)
	s.Require().NoError(err)
	s.Require().NoError(s.runProcmgr("stop", parControlProcess))
	s.waitForProcessState(parControlProcess, "Stopped", 30*time.Second)
	_ = s.runProcmgr("stop", parExecutorProcess)
	s.waitForProcessStates(parExecutorProcess, []string{"Created", "Skipped", "Stopped", "Exited", "Failed"}, 30*time.Second)
	s.Require().NoError(s.runProcmgr("start", parControlProcess))
}

// TestExecutorStartsForSignedWork proves the MSI's named-pipe mTLS path and
// generic Remote Config key delivery work with a real signed task.
func (s *windowsPARSplitLifecycleSuite) TestExecutorStartsForSignedWork() {
	client := s.Env().FakeIntake.Client()
	s.Require().NoError(client.FlushPAR())
	s.clearSigningKeys()
	s.T().Cleanup(s.clearSigningKeys)
	s.Require().NoError(client.RCAddConfig("", runnerKeysRCProduct, s.signingKey.id, s.signingKey.id, s.signingKey.config))

	s.waitForProcessState(parControlProcess, "Running", 2*time.Minute)
	// Go starts for configuration, then must idle out before signed work arrives.
	s.waitForProcessState(parExecutorProcess, "Exited", 3*time.Minute)

	setPARTaskSigningKey(s.T(), client, s.signingKey)
	taskID := uuid.New().String()
	s.Require().NoError(client.EnqueuePARTask(taskID, runCommandAction, map[string]interface{}{
		"command":         "echo windows-par-split-e2e",
		"allowedCommands": []string{"rshell:echo"},
	}))
	s.waitForProcessState(parExecutorProcess, "Running", 2*time.Minute)
	for range 5 {
		s.Require().NoError(client.RCAddConfig("", runnerKeysRCProduct, s.signingKey.id, s.signingKey.id, s.signingKey.config))
		time.Sleep(2 * time.Second)
	}
	result, err := client.GetPARTaskResult(taskID, 3*time.Minute)
	s.Require().NoError(err)
	s.Require().True(result.Success, "Windows split PAR action failed: %+v", result)
	s.Require().Contains(result.Outputs["stdout"], "windows-par-split-e2e")
	s.waitForProcessState(parExecutorProcess, "Exited", 2*time.Minute)
}

// TestExecutorPipeAdmitsOnlyIntendedAccounts checks the live executor pipe:
// its DACL is exactly the one the executor asks for, and an unrelated local
// user is denied even with par-control's narrowed client access. Together with
// TestExecutorStartsForSignedWork, which connects through par-control as the
// non-admin Agent account, this covers both sides of the pipe's access control.
func (s *windowsPARSplitLifecycleSuite) TestExecutorPipeAdmitsOnlyIntendedAccounts() {
	host := s.Env().RemoteHost
	tmp, err := host.GetTmpFolder()
	s.Require().NoError(err)
	scriptPath := host.JoinPath(tmp, "par-pipe-probe.ps1")
	_, err = host.WriteFile(scriptPath, pipeProbeScript)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = host.Remove(scriptPath) })

	// An ordinary local user: member of Users only, so it may log on locally.
	const probeUser = "parpipeprobe"
	probePassword := "Pp1!" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	_, err = host.Execute(fmt.Sprintf(
		`New-LocalUser -Name '%s' -Password (ConvertTo-SecureString '%s' -AsPlainText -Force) -AccountNeverExpires -PasswordNeverExpires | Out-Null; `+
			`Add-LocalGroupMember -SID 'S-1-5-32-545' -Member '%s'`,
		probeUser, probePassword, probeUser))
	s.Require().NoError(err)
	s.T().Cleanup(func() { _, _ = host.Execute(fmt.Sprintf(`Remove-LocalUser -Name '%s'`, probeUser)) })

	var probe pipeProbe
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		output, err := host.Execute(fmt.Sprintf(
			`& powershell.exe -NoProfile -ExecutionPolicy Bypass -File '%s' -ProcmgrCLI '%s' -ExecutorProcess '%s' -PipePath '%s' -ProbeUser '%s' -ProbePassword '%s' -ClientAccess %d`,
			scriptPath, s.procmgrCLI(), parExecutorProcess, parExecutorPipe, probeUser, probePassword, pipeClientAccess))
		if !assert.NoError(c, err) {
			return
		}
		lines := strings.Split(strings.TrimSpace(output), "\n")
		probe = pipeProbe{}
		if !assert.NoError(c, json.Unmarshal([]byte(strings.TrimSpace(lines[len(lines)-1])), &probe), output) {
			return
		}
		// The executor idled out between the two probes; try again.
		assert.NotEqual(c, errorFileNotFound, probe.ProbeError, "the executor pipe disappeared mid-probe")
	}, 3*time.Minute, 5*time.Second, "could not probe the executor pipe")

	s.T().Logf("executor pipe security descriptor: %s", probe.SDDL)
	s.Require().NotEmpty(probe.AgentSID)
	s.Require().Equal(probe.AgentSID, probe.ExecutorSID, "the executor must run as the Agent account")
	s.Require().False(probe.AgentIsAdmin,
		"the Agent account is an administrator, so this run cannot show the non-admin path works")

	// The OWNER RIGHTS entry applies to the owner, which must be the Agent
	// account for the executor to create further pipe instances.
	s.Equal(probe.AgentSID, probe.Owner, "the executor pipe should be owned by the Agent account")
	s.True(probe.Protected, "the pipe DACL must not inherit entries")
	grants := map[string]int64{}
	for _, ace := range probe.ACEs {
		s.Equal("AccessAllowed", ace.Type, "unexpected ACE for %s", ace.SID)
		grants[ace.SID] = ace.Mask
	}
	s.Equal(map[string]int64{
		"S-1-5-18":     pipeFullAccess,
		"S-1-5-32-544": pipeFullAccess,
		"S-1-3-4":      pipeServerAccess,
		probe.AgentSID: pipeClientAccess,
	}, grants)
	s.Equal(errorAccessDenied, probe.ProbeError, "an unrelated local user must be denied the executor pipe")
}

// TestMonolithStandsDown proves the monolithic runner exits cleanly instead of
// polling alongside the control plane. Its definition is auto_start, so a clean
// exit (not a crash) is the only correct outcome.
func (s *windowsPARSplitLifecycleSuite) TestMonolithStandsDown() {
	s.waitForProcessState(parMonolithProcess, "Exited", 2*time.Minute)
}

// TestControlStopsWithoutReenteringSupervisor proves par-control handles the
// CTRL_BREAK that dd-procmgrd sends for a graceful stop, instead of ignoring it
// and being force-killed once the 180s stop_timeout expires. The executor owns
// its idle shutdown independently of the control plane.
func (s *windowsPARSplitLifecycleSuite) TestControlStopsWithoutReenteringSupervisor() {
	defer func() {
		s.Require().NoError(s.runProcmgr("start", parControlProcess))
		s.waitForProcessState(parControlProcess, "Running", 2*time.Minute)
	}()

	started := time.Now()
	s.Require().NoError(s.runProcmgr("stop", parControlProcess))
	s.Require().Less(time.Since(started), 15*time.Second, "par-control stop should not hit its 180s timeout")

	s.waitForProcessState(parControlProcess, "Stopped", 10*time.Second)
	s.waitForProcessStates(parExecutorProcess, []string{"Created", "Skipped", "Stopped", "Exited"}, 3*time.Minute)
}

func (s *windowsPARSplitLifecycleSuite) clearSigningKeys() {
	client := s.Env().FakeIntake.Client()
	configs, err := client.RCListConfigs()
	s.Require().NoError(err)
	for _, config := range configs {
		if config.Product == runnerKeysRCProduct {
			key := fmt.Sprintf("%s/%s/%s/%s", config.OrgID, config.Product, config.ConfigID, config.ConfigName)
			s.Require().NoError(client.RCDeleteConfig(key))
		}
	}
}

func (s *windowsPARSplitLifecycleSuite) parControlBinary() string {
	return filepath.Join(s.installRoot, "bin", "agent", "par-control.exe")
}

func (s *windowsPARSplitLifecycleSuite) procmgrCLI() string {
	return filepath.Join(s.installRoot, "bin", "agent", "dd-procmgr.exe")
}

// runProcmgr shells out to the CLI, which reaches dd-procmgrd over the default
// named pipe.
func (s *windowsPARSplitLifecycleSuite) runProcmgr(command, name string) error {
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`& "%s" %s %s`, s.procmgrCLI(), command, name))
	return err
}

func (s *windowsPARSplitLifecycleSuite) waitForProcessState(name, state string, timeout time.Duration) {
	s.waitForProcessStates(name, []string{state}, timeout)
}

func (s *windowsPARSplitLifecycleSuite) waitForProcessStates(name string, states []string, timeout time.Duration) {
	s.T().Helper()
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		output, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`& "%s" describe %s`, s.procmgrCLI(), name))
		assert.NoError(c, err)
		output = strings.ReplaceAll(output, " ", "")
		assert.Condition(c, func() bool {
			for _, state := range states {
				if strings.Contains(output, "State:"+state) {
					return true
				}
			}
			return false
		}, "%s should be in one of these states: %v", name, states)
	}, timeout, 2*time.Second, "%s should enter one of these states: %v", name, states)
}

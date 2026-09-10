// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	_ "embed"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/cws/config"
)

// This reproduces the terrapin / Kata topology on a single host, which is
// possible because two processes on one box can talk over AF_VSOCK CID 2
// (VMADDR_CID_HOST) once vhost_vsock is loaded -- no VM required:
//
//	core agent + fakeintake        normal install
//	system-probe #1 (forwarder)    listens on vsock 2:5020, ships to the intake
//	system-probe #2 (guest)        dials 2:5020, reaches nothing else
//
// The two roles match the vsock modes added in datadog-operator#3186: the
// forwarder is "SystemProbe" mode, the guest is "Full" mode. That PR's own unit
// tests cover whether the operator emits these env vars; what needs real infra,
// and has no coverage anywhere today, is whether the Agent then actually works.

const (
	// vsockEventPort is the runtime-security event port both roles agree on.
	vsockEventPort = 5020

	// guestUnit is a transient systemd unit, so the guest survives the SSH
	// session that starts it and can be stopped again without reprovisioning.
	guestUnit = "datadog-sysprobe-guest"

	systemProbeBin = "/opt/datadog-agent/embedded/bin/system-probe"

	// The guest writes everything under /tmp to avoid colliding with the
	// forwarder. Its own log file is what makes the assertions below
	// unambiguous: nothing the forwarder logs can satisfy them.
	guestLogPath    = "/tmp/system-probe.log"
	guestEnvPath    = "/tmp/sysprobe-guest.env"
	guestConfigPath = "/tmp/sysprobe-guest.yaml"

	forwarderLogPath = "/var/log/datadog/system-probe.log"

	// guestHostname must match DD_HOSTNAME in the env fixture.
	guestHostname = "terrapin"
)

//go:embed config/e2e-vsock-forwarder-system-probe.yaml
var vsockForwarderSystemProbeConfig string

//go:embed config/e2e-vsock-forwarder-security-agent.yaml
var vsockForwarderSecurityAgentConfig string

//go:embed config/e2e-vsock-guest.env
var vsockGuestEnv string

// vsockUserData loads vhost_vsock before the Agent is installed. It cannot wait
// until SetupSuite: the forwarder binds vsock at startup and CWSConsumer.Start
// returns the bind error, so an Agent that starts without the module never comes
// up at all.
const vsockUserData = `#!/bin/bash
modprobe vhost_vsock
echo vhost_vsock > /etc/modules-load.d/datadog-e2e-vsock.conf
`

// The guest is configured entirely through DD_* variables; this exists only so it
// does not inherit the forwarder's system-probe.yaml, which would otherwise make
// it a second event *server*.
const guestConfigYAML = "system_probe_config:\n  enabled: true\n"

type vsockForwardingSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestVSockForwardingSuite(t *testing.T) {
	testID := uuid.NewString()[:4]
	agentConfig := config.GenDatadogAgentConfig("cws-e2e-vsock-fwd-"+testID, "tag1", "tag2")

	e2e.Run(t, &vsockForwardingSuite{},
		e2e.WithStackName("cws-vsock-forwarding"),
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(
					ec2.WithInternetAccess(),
					ec2.WithUserData(vsockUserData),
				),
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(agentConfig),
					agentparams.WithSystemProbeConfig(vsockForwarderSystemProbeConfig),
					agentparams.WithSecurityAgentConfig(vsockForwarderSecurityAgentConfig),
				),
			),
		)),
	)
}

func (s *vsockForwardingSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	host := s.Env().RemoteHost

	// Fail loudly rather than skipping: a silent skip would drop this coverage
	// the day an AMI stops shipping the module, and nobody would notice.
	out, err := host.Execute("lsmod | grep -c vhost_vsock || true")
	require.NoError(s.T(), err)
	require.NotEqual(s.T(), "0", strings.TrimSpace(out),
		"vhost_vsock is not loaded, so no process can bind vsock CID 2 -- the user data should have loaded it")

	// SFTP rather than a heredoc over SSH: no shell quoting to get wrong, and
	// /tmp is writable by the login user so this needs no sudo. systemd-run
	// still reads them as root, which 0644 allows.
	_, err = host.WriteFile(guestEnvPath, []byte(vsockGuestEnv))
	require.NoError(s.T(), err, "could not write the guest env file")
	_, err = host.WriteFile(guestConfigPath, []byte(guestConfigYAML))
	require.NoError(s.T(), err, "could not write the guest config file")

	s.T().Logf("starting the guest system-probe as transient unit %s", guestUnit)
	host.MustExecute(fmt.Sprintf(
		"sudo systemd-run --unit=%s --collect --property=EnvironmentFile=%s %s run --config=%s",
		guestUnit, guestEnvPath, systemProbeBin, guestConfigPath))

	// systemd-run returns as soon as the job is queued, so confirm the unit
	// actually runs. Without this a guest that dies on startup -- two
	// system-probes loading CWS eBPF on one kernel is the plausible cause --
	// shows up as five assertion timeouts instead of one legible failure.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		state := strings.TrimSpace(host.MustExecuteOn(c,
			"systemctl is-active "+guestUnit+" || true"))
		assert.Equal(c, "active", state, "guest unit is %q, not active", state)
	}, 90*time.Second, 5*time.Second,
		"the guest system-probe never reached active; see the journal in the diagnostics below")
}

func (s *vsockForwardingSuite) TearDownSuite() {
	// Stop the guest before the stack goes away so a retry on the same infra
	// starts from a clean slate.
	if env := s.Env(); env != nil && env.RemoteHost != nil {
		host := env.RemoteHost
		if _, err := host.Execute("sudo systemctl stop " + guestUnit + " || true"); err != nil {
			s.T().Logf("could not stop %s: %v", guestUnit, err)
		}
	}
	s.BaseSuite.TearDownSuite()
}

// TestForwarderListensOnVSock checks the "SystemProbe" vsock role wired a real
// listener: event_grpc_server=system-probe plus socket=vsock:5020 must produce a
// bound CID 2 socket, not just a log line.
func (s *vsockForwardingSuite) TestForwarderListensOnVSock() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(c, "sudo ss -a --vsock || true")
		assert.Contains(c, out, fmt.Sprintf("2:%d", vsockEventPort),
			"the forwarder is not listening on vsock 2:%d", vsockEventPort)
	}, 3*time.Minute, 10*time.Second)

	assertLogsEventually(s.T(), s, forwarderLogPath, []string{
		fmt.Sprintf("starting system-probe remote event server on vsock:%d", vsockEventPort),
	}, 2*time.Minute)
}

// TestGuestConnectsOverVSock checks the guest picked the vsock transport and got
// an accepted stream. "api client connected" is only logged once SendEvents has
// a working stream to the forwarder, so it cannot pass while the hop is broken.
func (s *vsockForwardingSuite) TestGuestConnectsOverVSock() {
	assertLogsEventually(s.T(), s, guestLogPath, []string{
		fmt.Sprintf("connecting to security agent via socket: vsock:%d", vsockEventPort),
		"using socket family 'vsock'",
		"api client connected, starts sending events",
	}, 4*time.Minute)
}

// TestEventCrossesVSockHop is the end-to-end assertion. RemoteEventServer logs
// this line on the receiving side for each event it accepts, so it is direct
// evidence that a CWS event produced inside the guest travelled over vsock into
// the forwarder. CWS self-tests generate the events on guest startup.
func (s *vsockForwardingSuite) TestEventCrossesVSockHop() {
	assertLogsEventually(s.T(), s, forwarderLogPath, []string{
		"received remote event from rule",
	}, 6*time.Minute)
}

// TestGuestIsIsolatedFromCoreAgent checks the guest reached nothing but the
// forwarder. Absence cannot be waited for, so this first waits for proof the
// guest got far enough to have tried -- subtests must not depend on ordering.
func (s *vsockForwardingSuite) TestGuestIsIsolatedFromCoreAgent() {
	assertLogsEventually(s.T(), s, guestLogPath, []string{
		"api client connected, starts sending events",
	}, 4*time.Minute)

	logs, err := s.Env().RemoteHost.ReadFilePrivileged(guestLogPath)
	require.NoError(s.T(), err)

	// Enabled-path success markers, one per component the guest must not bring
	// up, plus the retry messages that made this switch necessary.
	for _, unwanted := range []string{
		"remote workloadmeta initialized successfully",
		"remote tagger initialized successfully",
		"remote workloadfilter initialized successfully",
		"Registered with Remote Agent Registry",
		"waiting for initial configuration",
		"unable to establish stream, will possibly retry",
		"error received trying to start stream",
	} {
		assert.NotContains(s.T(), string(logs), unwanted,
			"the guest contacted the core agent; it should only ever reach the forwarder over vsock")
	}
}

// TestGuestReportsItsOwnHostname guards the discriminator the other assertions
// rely on: guest payloads are only attributable because it runs with a distinct
// DD_HOSTNAME.
func (s *vsockForwardingSuite) TestGuestReportsItsOwnHostname() {
	assertLogsEventually(s.T(), s, guestLogPath, []string{guestHostname}, 4*time.Minute)
}

// assertLogsEventually waits until every pattern appears in a log file on the
// remote host, reporting which are still missing on failure.
func assertLogsEventually(t *testing.T, s *vsockForwardingSuite, path string, expected []string, waitFor time.Duration) {
	t.Helper()

	found := make(map[string]bool, len(expected))
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		content, err := s.Env().RemoteHost.ReadFilePrivileged(path)
		require.NoError(c, err)
		logs := string(content)

		missing := make([]string, 0, len(expected))
		for _, want := range expected {
			if found[want] || strings.Contains(logs, want) {
				found[want] = true
				continue
			}
			missing = append(missing, want)
		}
		assert.Empty(c, missing, "still missing from %s", path)
	}, waitFor, 10*time.Second, "expected lines never appeared in %s", path)
}

// AfterTest captures what a debug run would need, since CI destroys the host.
func (s *vsockForwardingSuite) AfterTest(suiteName, testName string) {
	s.BaseSuite.AfterTest(suiteName, testName)

	if !s.T().Failed() {
		return
	}

	diagnostics := []struct {
		what string
		cmd  string
	}{
		{"vsock sockets", "sudo ss -a --vsock || true"},
		{"vsock modules", "lsmod | grep -E 'vsock|vhost' || true"},
		{"guest unit", "sudo systemctl status " + guestUnit + " --no-pager || true"},
		{"guest journal", "sudo journalctl -u " + guestUnit + " --no-pager | tail -n 40 || true"},
		{"guest log", "sudo tail -n 60 " + guestLogPath + " || true"},
		{"forwarder log (vsock / remote event lines)",
			"sudo grep -n -E 'vsock|remote event|event server|runtime security' " + forwarderLogPath + " | tail -n 60 || true"},
	}

	env := s.Env()
	if env == nil || env.RemoteHost == nil {
		return
	}

	host := env.RemoteHost
	for _, d := range diagnostics {
		out, err := host.Execute(d.cmd)
		if err != nil {
			s.T().Logf("diagnostic %q failed: %v", d.what, err)
			continue
		}
		s.T().Logf("%s:\n%s", d.what, out)
	}
}

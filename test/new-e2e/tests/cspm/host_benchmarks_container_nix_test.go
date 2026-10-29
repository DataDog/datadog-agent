// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	ec2docker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
)

// containerBenchmarksSuite runs the host benchmarks with the agent in a container
// inspecting the host root mounted at /host, using the pipeline-built agent image.
type containerBenchmarksSuite struct {
	e2e.BaseSuite[environments.DockerHost]
	distro distro
}

func testContainerizedHostBenchmarks(t *testing.T, d distro) {
	t.Parallel()
	vmOS := d.os
	if d.containerOS != nil {
		vmOS = *d.containerOS
	}
	vmOpts := []ec2.VMOption{ec2.WithOS(vmOS)}
	e2e.Run(t, &containerBenchmarksSuite{distro: d},
		e2e.WithStackName("cspm-container-"+d.name),
		e2e.WithProvisioner(awsdocker.Provisioner(awsdocker.WithRunOptions(
			ec2docker.WithEC2VMOptions(vmOpts...),
			ec2docker.WithAgentOptions(
				dockeragentparams.WithExtraVolumes("/:/host:ro"),
				dockeragentparams.WithAgentServiceEnvVariable("HOST_ROOT", pulumi.String("/host")),
				dockeragentparams.WithAgentServiceEnvVariable("DD_COMPLIANCE_CONFIG_ENABLED", pulumi.String("true")),
				dockeragentparams.WithAgentServiceEnvVariable("DD_COMPLIANCE_CONFIG_HOST_BENCHMARKS_ENABLED", pulumi.String("true")),
			),
		))),
	)
}

// No TestContainerizedHostBenchmarksRHEL8: ec2docker's docker-CE install is unreliable on
// RHEL 8 (el8 docker 26 vs the daemon's nftables firewall-backend), and the RHEL 9 container
// already covers the rhel-family containerized path. RHEL 8 is host-only.

func TestContainerizedHostBenchmarksRHEL9(t *testing.T) {
	testContainerizedHostBenchmarks(t, distroRHEL9)
}

func TestContainerizedHostBenchmarksRHEL10(t *testing.T) {
	testContainerizedHostBenchmarks(t, distroRHEL10)
}

func TestContainerizedHostBenchmarksUbuntu2404(t *testing.T) {
	testContainerizedHostBenchmarks(t, distroUbuntu2404)
}

func TestContainerizedHostBenchmarksAmazonLinux2023(t *testing.T) {
	testContainerizedHostBenchmarks(t, distroAmazonLinux2023)
}

func TestContainerizedHostBenchmarksAlmaLinux9(t *testing.T) {
	testContainerizedHostBenchmarks(t, distroAlmaLinux9)
}

func (s *containerBenchmarksSuite) runHost(cmd string) string {
	return s.Env().RemoteHost.MustExecute(cmd)
}

func (s *containerBenchmarksSuite) runAgent(cmd string) string {
	return s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "sh", "-c", cmd)
}

func (s *containerBenchmarksSuite) check(args string) []event {
	cmd := fmt.Sprintf("%s compliance check %s 2>/dev/null", securityAgent, args)
	return parseEvents(s.T(), s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "sh", "-c", cmd))
}

// fipsCheck returns a check that runs compliance check with tmp as its TMPDIR,
// libgcrypt forced into FIPS mode and a memory lock limit of zero, as on a
// FIPS host in system-probe, whose seccomp profile denies mlock.
func (s *containerBenchmarksSuite) fipsCheck(tmp string) func(string) []event {
	return func(args string) []event {
		cmd := fmt.Sprintf("ulimit -l 0 && TMPDIR=%s LIBGCRYPT_FORCE_FIPS_MODE=1 %s compliance check %s 2>/dev/null", tmp, securityAgent, args)
		return parseEvents(s.T(), s.runAgent(cmd))
	}
}

// TestFIPSWithoutLockedMemory checks that the probe rules give the results of a
// normal run in libgcrypt's FIPS mode with no memory to lock, and that those
// runs leave their TMPDIR empty. They get a TMPDIR of their own: the agent's
// scheduled benchmarks keep their temporary directories in /tmp while they
// run.
func (s *containerBenchmarksSuite) TestFIPSWithoutLockedMemory() {
	tmp := strings.TrimSpace(s.runAgent("mktemp -d /tmp/cspm-fips.XXXXXX"))
	require.NotEmpty(s.T(), tmp, "creating the TMPDIR of the FIPS runs")
	defer s.runAgent("rm -rf " + tmp)
	fipsCheck := s.fipsCheck(tmp)
	for _, p := range probesFor(s.distro, false) {
		want := checkRule(s.T(), s.distro, s.check, p.rule).Result
		got := checkRule(s.T(), s.distro, fipsCheck, p.rule).Result
		assert.Equalf(s.T(), want, got, "%s in FIPS mode without locked memory", p.rule)
	}
	assert.Empty(s.T(), strings.Fields(s.runAgent("ls -A "+tmp)), "entries left in the TMPDIR of the FIPS runs")
}

// TestBundledContent checks the agent image ships this distro's benchmark content.
func (s *containerBenchmarksSuite) TestBundledContent() {
	listing := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "sh", "-c", "ls /etc/datadog-agent/compliance.d/")
	assertBundledContent(s.T(), s.distro, listing)
}

func (s *containerBenchmarksSuite) TestConsistency() {
	assertConsistency(s.T(), s.distro, false, s.check)
}

func (s *containerBenchmarksSuite) TestProbes() {
	for _, p := range probesFor(s.distro, false) {
		s.Run(p.name, func() { runProbe(s.T(), s.distro, p, s.runHost, s.check) })
	}
}

// TestDeterminism re-runs the whole benchmark in the container and asserts identical
// results, catching non-deterministic rules on the containerized path.
func (s *containerBenchmarksSuite) TestDeterminism() {
	first := resultsByRule(s.distro, s.check)
	assert.Equal(s.T(), first, resultsByRule(s.distro, s.check),
		"benchmark results differ between identical runs")
}

// TestReporting checks the containerized agent's findings reach the backend. The docker
// fakeintake provisioner redirects compliance_config.endpoints, and findings ride the logs
// pipeline, so a --report run lands them in fakeintake.
func (s *containerBenchmarksSuite) TestReporting() {
	// Bound reporting when fakeintake is unreachable and preserve stderr for diagnosis.
	output, err := s.Env().Docker.Client.ExecuteCommandWithErr(s.Env().Agent.ContainerName,
		"timeout", "--kill-after=10s", "2m", securityAgent, "compliance", "check", "--report")
	require.NoErrorf(s.T(), err,
		"compliance reporting failed (2m deadline, forced kill after another 10s); check container-to-fakeintake connectivity; output: %s", output)
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		findings, err := s.Env().FakeIntake.Client().GetComplianceFindings()
		require.NoError(c, err)
		reported := false
		for _, f := range findings {
			if f.FrameworkID == s.distro.frameworkID {
				reported = true
				break
			}
		}
		assert.Truef(c, reported, "no %s findings reached fakeintake (%d total)", s.distro.frameworkID, len(findings))
	}, 5*time.Minute, 15*time.Second)
}

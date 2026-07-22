// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
	"fmt"
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
	if d.latestAMI {
		vmOpts = append(vmOpts, ec2.WithLatestAMI())
	}
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

func (s *containerBenchmarksSuite) check(args string) []event {
	cmd := fmt.Sprintf("%s compliance check %s 2>/dev/null", securityAgent, args)
	return parseEvents(s.T(), s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "sh", "-c", cmd))
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
	cmd := securityAgent + " compliance check --report 2>/dev/null"
	s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "sh", "-c", cmd)
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

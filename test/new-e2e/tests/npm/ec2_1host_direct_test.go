// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
)

type ec2VMDirectSuite struct {
	e2e.BaseSuite[hostHttpbinEnv]
}

// TestEC2VMSuite will validate running the agent on a single EC2 VM
func TestEC2VMDirectSuite(t *testing.T) {
	t.Parallel()
	s := &ec2VMDirectSuite{}

	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(provisioners.NewTypedPulumiProvisioner(
		"hostHttpbin",
		hostDockerHttpbinEnvProvisioner(ec2.WithAgentOptions(agentparams.WithSystemProbeConfig(systemProbeConfigNPMDirect))),
		nil,
	))}

	// Source of our E2E CI images test/new-e2e/tests/agent-platform/platforms.json
	// Other VM image can be used, our E2E CI images test/new-e2e/tests/agent-platform/platforms.json
	// ec2params.WithImageName("ami-a4dc46db", os.AMD64Arch, ec2os.AmazonLinuxOS) // ubuntu-16-04-4.4
	e2e.Run(t, s, e2eParams...)
}

func (v *ec2VMDirectSuite) SetupSuite() {
	v.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer v.CleanupOnSetupFailure()

	v.Env().RemoteHost.MustExecute("sudo apt install -y apache2-utils docker.io")
	v.Env().RemoteHost.MustExecute("sudo usermod -a -G docker ubuntu")
	v.Env().RemoteHost.Reconnect()

	// prefetch docker image locally
	v.Env().RemoteHost.MustExecute("docker pull ghcr.io/datadog/apps-npm-tools:" + apps.Version)
}

// BeforeTest will be called before each test
func (v *ec2VMDirectSuite) BeforeTest(suiteName, testName string) {
	v.BaseSuite.BeforeTest(suiteName, testName)

	// Verify that the process agent is not running
	assert.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		status := v.Env().RemoteHost.MustExecute("sudo /opt/datadog-agent/embedded/bin/process-agent status")
		assert.Contains(c, status, "The Process Agent is not running")
	}, 1*time.Minute, 5*time.Second)

	// default is to reset the current state of the fakeintake aggregators
	if !v.BaseSuite.IsDevMode() {
		v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// AfterTest will be called after each test
func (v *ec2VMDirectSuite) AfterTest(suiteName, testName string) {
	test1HostFakeIntakeNPMDumpInfo(v.T(), v.Env().FakeIntake)

	v.BaseSuite.AfterTest(suiteName, testName)
}

// TestFakeIntakeNPM_HostRequests Validate the agent can communicate with the (fake) backend and send connections every 30 seconds
// 2 tests generate the request on the host and on docker
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
//
// The test start by 00 to validate the agent/system-probe is up and running
func (v *ec2VMDirectSuite) Test00FakeIntakeNPM_HostRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	// generate a connection
	v.Env().RemoteHost.MustExecute("curl " + testURL)

	test1HostFakeIntakeNPM(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM_DockerRequests Validate the agent can communicate with the (fake) backend and send connections every 30 seconds
// 2 tests generate the request on the host and on docker
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
func (v *ec2VMDirectSuite) TestFakeIntakeNPM_DockerRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	// generate a connection
	v.Env().RemoteHost.MustExecute("docker run --rm ghcr.io/datadog/apps-npm-tools:" + apps.Version + " curl " + testURL)

	test1HostFakeIntakeNPM(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM600cnxBucket_HostRequests Validate the agent can communicate with the (fake) backend and send connections
// every 30 seconds with a maximum of 600 connections per payloads, if more another payload will follow.
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for n payloads and check if the last 2 have a maximum span of 200ms
func (v *ec2VMDirectSuite) TestFakeIntakeNPM600cnxBucket_HostRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	// generate connections
	v.Env().RemoteHost.MustExecute("ab -n 1500 -c 600 " + testURL)

	test1HostFakeIntakeNPM600cnxBucket(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM600cnxBucket_DockerRequests Validate the agent can communicate with the (fake) backend and send connections
// every 30 seconds with a maximum of 600 connections per payloads, if more another payload will follow.
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for n payloads and check if the last 2 have a maximum span of 200ms
func (v *ec2VMDirectSuite) TestFakeIntakeNPM600cnxBucket_DockerRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	// generate connections
	v.Env().RemoteHost.MustExecute("docker run --rm ghcr.io/datadog/apps-npm-tools:" + apps.Version + " ab -n 1500 -c 600 " + testURL)

	test1HostFakeIntakeNPM600cnxBucket(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM_TCP_UDP_DNS_HostRequests validate we received tcp, udp, and DNS connections
// with some basic checks, like IPs/Ports present, DNS query has been captured, ...
func (v *ec2VMDirectSuite) TestFakeIntakeNPM_TCP_UDP_DNS_HostRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	// generate connections
	v.Env().RemoteHost.MustExecute("curl " + testURL)
	v.Env().RemoteHost.MustExecute("dig @8.8.8.8 www.google.ch")

	test1HostFakeIntakeNPMTCPUDPDNS(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM_TCP_UDP_DNS_DockerRequests validate we received tcp, udp, and DNS connections
// with some basic checks, like IPs/Ports present, DNS query has been captured, ...
func (v *ec2VMDirectSuite) TestFakeIntakeNPM_TCP_UDP_DNS_DockerRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	// generate connections
	v.Env().RemoteHost.MustExecute("docker run --rm ghcr.io/datadog/apps-npm-tools:" + apps.Version + " curl " + testURL)
	v.Env().RemoteHost.MustExecute("docker run --rm ghcr.io/datadog/apps-npm-tools:" + apps.Version + " dig @8.8.8.8 www.google.ch")

	test1HostFakeIntakeNPMTCPUDPDNS(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM_ResolvConf_DockerRequests validates that connections from Docker
// containers include resolv.conf data.
func (v *ec2VMDirectSuite) TestFakeIntakeNPM_ResolvConf_DockerRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	// generate a connection from a Docker container
	v.Env().RemoteHost.MustExecute("docker run --rm ghcr.io/datadog/apps-npm-tools:" + apps.Version + " curl " + testURL)

	test1HostFakeIntakeNPMResolvConf(&v.BaseSuite, v.Env().FakeIntake)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	compos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
)

type ec2VMSELinuxSuite struct {
	e2e.BaseSuite[hostHttpbinEnv]
}

// TestEC2VMSuite will validate running the agent on a single EC2 VM
func TestEC2VMSELinuxSuite(t *testing.T) {
	t.Parallel()
	s := &ec2VMSELinuxSuite{}

	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(
		provisioners.NewTypedPulumiProvisioner("hostHttpbin", hostDockerHttpbinEnvProvisioner(ec2.WithEC2InstanceOptions(
			// RHEL9
			ec2.WithAMI(
				"ami-04e7f0e0bde783f77", // https://gitlab.ddbuild.io/DataDog/ami-builder/-/jobs/1232214462
				compos.AmazonLinux2,
				compos.AMD64Arch,
			),
			ec2.WithInstanceType("t3.medium"))), nil)),
	}

	// Source of our kitchen CI images test/kitchen/platforms.json
	// Other VM image can be used, our kitchen CI images test/kitchen/platforms.json
	// ec2params.WithImageName("ami-a4dc46db", os.AMD64Arch, ec2os.AmazonLinuxOS) // ubuntu-16-04-4.4
	e2e.Run(t, s, e2eParams...)
}

// BeforeTest will be called before each test
func (v *ec2VMSELinuxSuite) BeforeTest(suiteName, testName string) {
	v.BaseSuite.BeforeTest(suiteName, testName)

	// default is to reset the current state of the fakeintake aggregators
	if !v.BaseSuite.IsDevMode() {
		v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// AfterTest will be called after each test
func (v *ec2VMSELinuxSuite) AfterTest(suiteName, testName string) {
	test1HostFakeIntakeNPMDumpInfo(v.T(), v.Env().FakeIntake)

	v.BaseSuite.AfterTest(suiteName, testName)
}

func (v *ec2VMSELinuxSuite) SetupSuite() {
	v.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer v.CleanupOnSetupFailure()

	v.Env().RemoteHost.MustExecute("sudo usermod -a -G docker $(whoami)")
	v.Env().RemoteHost.Reconnect()

	// prefetch docker image locally
	v.Env().RemoteHost.MustExecute("docker pull ghcr.io/datadog/apps-npm-tools:" + apps.Version)
}

// TestFakeIntakeNPM_HostRequests Validate the agent can communicate with the (fake) backend and send connections every 30 seconds
// 2 tests generate the request on the host and on docker
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
//
// The test start by 00 to validate the agent/system-probe is up and running
func (v *ec2VMSELinuxSuite) Test00FakeIntakeNPM_HostRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	// generate a connection
	v.Env().RemoteHost.MustExecute("curl " + testURL)

	test1HostFakeIntakeNPM(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM_DockerRequests Validate the agent can communicate with the (fake) backend and send connections every 30 seconds
// 2 tests generate the request on the host and on docker
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
func (v *ec2VMSELinuxSuite) TestFakeIntakeNPM_DockerRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	// generate a connection
	v.Env().RemoteHost.MustExecute("docker run --rm ghcr.io/datadog/apps-npm-tools:" + apps.Version + " curl " + testURL)

	test1HostFakeIntakeNPM(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM600cnxBucket_HostRequests Validate the agent can communicate with the (fake) backend and send connections
// every 30 seconds with a maximum of 600 connections per payloads, if more another payload will follow.
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for n payloads and check if the last 2 have a maximum span of 200ms
func (v *ec2VMSELinuxSuite) TestFakeIntakeNPM600cnxBucket_HostRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	// generate connections
	v.Env().RemoteHost.MustExecute("ab -n 600 -c 600 " + testURL)

	test1HostFakeIntakeNPM600cnxBucket(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM600cnxBucket_DockerRequests Validate the agent can communicate with the (fake) backend and send connections
// every 30 seconds with a maximum of 600 connections per payloads, if more another payload will follow.
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for n payloads and check if the last 2 have a maximum span of 200ms
func (v *ec2VMSELinuxSuite) TestFakeIntakeNPM600cnxBucket_DockerRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	// generate connections
	v.Env().RemoteHost.MustExecute("docker run --rm ghcr.io/datadog/apps-npm-tools:" + apps.Version + " ab -n 600 -c 600 " + testURL)

	test1HostFakeIntakeNPM600cnxBucket(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM_TCP_UDP_DNS_HostRequests validate we received tcp, udp, and DNS connections
// with some basic checks, like IPs/Ports present, DNS query has been captured, ...
func (v *ec2VMSELinuxSuite) TestFakeIntakeNPM_TCP_UDP_DNS_HostRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	// generate connections
	v.Env().RemoteHost.MustExecute("curl " + testURL)
	v.Env().RemoteHost.MustExecute("dig @8.8.8.8 www.google.ch")

	test1HostFakeIntakeNPMTCPUDPDNS(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM_TCP_UDP_DNS_DockerRequests validate we received tcp, udp, and DNS connections
// with some basic checks, like IPs/Ports present, DNS query has been captured, ...
func (v *ec2VMSELinuxSuite) TestFakeIntakeNPM_TCP_UDP_DNS_DockerRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	// generate connections
	v.Env().RemoteHost.MustExecute("docker run --rm ghcr.io/datadog/apps-npm-tools:" + apps.Version + " curl " + testURL)
	v.Env().RemoteHost.MustExecute("docker run --rm ghcr.io/datadog/apps-npm-tools:" + apps.Version + " dig @8.8.8.8 www.google.ch")

	test1HostFakeIntakeNPMTCPUDPDNS(&v.BaseSuite, v.Env().FakeIntake)
}

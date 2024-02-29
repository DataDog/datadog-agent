// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/activedirectory"
	platformCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	TestDomain   = "datadogqalab.com"
	TestUser     = "TestUser"
	TestPassword = "Test1234#"
)

type ec2VMWKitSuite struct {
	windows.BaseAgentInstallerSuite[activedirectory.Env]
}

// TestEC2VMWKitSuite will validate running the agent on a single EC2 VM
func TestEC2VMWKitSuite(t *testing.T) {
	s := &ec2VMWKitSuite{}

	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(activedirectory.Provisioner(
		activedirectory.WithActiveDirectoryOptions(
			activedirectory.WithDomainName(TestDomain),
			activedirectory.WithDomainPassword(TestPassword),
			activedirectory.WithDomainUser(TestUser, TestPassword),
		)))}

	e2e.Run(t, s, e2eParams...)
}

// BeforeTest will be called before each test
func (v *ec2VMWKitSuite) BeforeTest(suiteName, testName string) {
	v.BaseAgentInstallerSuite.BeforeTest(suiteName, testName)

	// default is to reset the current state of the fakeintake aggregators
	if !v.BaseSuite.IsDevMode() {
		v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

func (v *ec2VMWKitSuite) SetupSuite() {
	v.BaseAgentInstallerSuite.SetupSuite()

	host := v.Env().DomainControllerHost

	_, err := v.InstallAgent(host,
		windowsAgent.WithPackage(v.AgentPackage),
		windowsAgent.WithAgentUser(fmt.Sprintf("%s\\%s", TestDomain, TestUser)),
		windowsAgent.WithAgentUserPassword(fmt.Sprintf("\"%s\"", TestPassword)),
		windowsAgent.WithValidAPIKey(),
		windowsAgent.WithFakeIntake(v.Env().FakeIntake),
		windowsAgent.WithInstallLogFile("TC-INS-DC-006_install.log"))

	v.Require().NoError(err, "should succeed to install Agent on a Domain Controller with a valid domain account & password")

	tc := v.NewTestClientForHost(host)
	tc.CheckAgentVersion(v.T(), v.AgentPackage.AgentVersion())
	platformCommon.CheckAgentBehaviour(v.T(), tc)
	v.EventuallyWithT(func(c *assert.CollectT) {
		stats, err := v.Env().FakeIntake.Client().RouteStats()
		assert.NoError(c, err)
		assert.NotEmpty(c, stats)
	}, 5*time.Minute, 10*time.Second)

	// v.Env().RemoteHost.MustExecute("sudo yum install -y bind-utils httpd-tools")
	// v.Env().RemoteHost.MustExecute("sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo")
	// v.Env().RemoteHost.MustExecute("sudo yum install -y docker-ce docker-ce-cli")
	// v.Env().RemoteHost.MustExecute("sudo systemctl start docker")
	// v.Env().RemoteHost.MustExecute("sudo usermod -a -G docker $(whoami)")
	// v.Env().RemoteHost.ReconnectSSH()
	//
	// // prefetch docker image locally
	// v.Env().RemoteHost.MustExecute("docker pull public.ecr.aws/k8m1l3p1/alpine/curler:latest")
	// v.Env().RemoteHost.MustExecute("docker pull public.ecr.aws/docker/library/httpd:latest")
	// v.Env().RemoteHost.MustExecute("docker pull public.ecr.aws/patrickc/troubleshoot-util:latest")
}

// TestFakeIntakeNPM_HostRequests Validate the agent can communicate with the (fake) backend and send connections every 30 seconds
// 2 tests generate the request on the host and on docker
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
func (v *ec2VMWKitSuite) TestFakeIntakeNPM_HostRequests() {
	//testURL := "http://" // + v.Env().HTTPBinHost.Address + "/"

	// generate a connection
	//v.Env().RemoteHost.MustExecute("curl " + testURL)

	host := v.Env().DomainControllerHost
	host.MustExecute("$result = Invoke-WebRequest -UseBasicParsing -Uri http://httpbin.org")
	// $rss = Invoke-WebRequest -Uri http://httpbin.org -UseBasicParsing

	test1HostFakeIntakeNPM(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM600cnxBucket_HostRequests Validate the agent can communicate with the (fake) backend and send connections
// every 30 seconds with a maximum of 600 connections per payloads, if more another payload will follow.
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for n payloads and check if the last 2 have a maximum span of 100ms
func (v *ec2VMWKitSuite) TestFakeIntakeNPM600cnxBucket_HostRequests() {
	//testURL := "http://" //+ v.Env().HTTPBinHost.Address + "/"

	// generate connections
	//v.Env().RemoteHost.MustExecute("ab -n 600 -c 600 " + testURL)

	// test1HostFakeIntakeNPM600cnxBucket(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM_TCP_UDP_DNS_HostRequests validate we received tcp, udp, and DNS connections
// with some basic checks, like IPs/Ports present, DNS query has been captured, ...
func (v *ec2VMWKitSuite) TestFakeIntakeNPM_TCP_UDP_DNS_HostRequests() {
	//testURL := "http://" //+ v.Env().HTTPBinHost.Address + "/"

	// generate connections
	//v.Env().RemoteHost.MustExecute("curl " + testURL)
	//v.Env().RemoteHost.MustExecute("dig @8.8.8.8 www.google.ch")

	// test1HostFakeIntakeNPMTCPUDPDNS(&v.BaseSuite, v.Env().FakeIntake)
}

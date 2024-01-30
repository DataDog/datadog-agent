// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"math"
	"os"
	"testing"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/docker"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"

	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
	compos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/stretchr/testify/assert"
)

type dockerHostNginxEnv struct {
	//	environments.DockerHost
	// Components
	RemoteHost *components.RemoteHost
	FakeIntake *components.FakeIntake
	Agent      *components.DockerAgent

	// Other clients
	Docker *client.Docker

	// Components
	NginxHost *components.RemoteHost

	// Other clients
	//	Docker *client.Docker
}

type ec2VMContainerizedSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func dockerHostNginxEnvProvisioner() e2e.PulumiEnvRunFunc[environments.DockerHost] {
	return func(ctx *pulumi.Context, env *environments.DockerHost) error {
		vmName := "nginxvm"
		vmOptions := []ec2.VMOption{ec2.WithOS(compos.UbuntuDefault)}

		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		host, err := ec2.NewVM(awsEnv, vmName, vmOptions...)
		if err != nil {
			return err
		}

		host = host

		/*
				err = host.Export(ctx, &NginxHost.HostOutput)
				if err != nil {
					return err
				}


			fmt.Printf("====== NginxHost.HostOutput.Address: %v\n", NginxHost.HostOutput.Address)
		*/
		/*
			manager, _, err := docker.NewManager(*awsEnv.CommonEnvironment, host, true)
			if err != nil {
				return err
			}
			manager = manager
		*/

		return nil
	}
}

// TestEC2VMSuite will validate running the agent on a single EC2 VM
func TestEC2VMContainerizedSuite(t *testing.T) {
	s := &ec2VMContainerizedSuite{}

	e2eParams := []e2e.SuiteOption{}
	e2eParams1 := []e2e.SuiteOption{e2e.WithProvisioner(awsdocker.Provisioner(awsdocker.WithAgentOptions(
		dockeragentparams.WithAgentServiceEnvVariable("DD_SYSTEM_PROBE_NETWORK_ENABLED", pulumi.StringPtr("true")),
	)))}

	// adding nginx
	e2eParams2 := []e2e.SuiteOption{e2e.WithPulumiProvisioner(dockerHostNginxEnvProvisioner(), nil)}

	e2eParams = append(e2eParams, e2eParams1...)
	e2eParams = append(e2eParams, e2eParams2...)

	// debug helper
	if _, devmode := os.LookupEnv("TESTS_E2E_DEVMODE"); devmode {
		e2eParams = append(e2eParams, e2e.WithDevMode())
	}

	// Source of our kitchen CI images test/kitchen/platforms.json
	// Other VM image can be used, our kitchen CI images test/kitchen/platforms.json
	// ec2params.WithImageName("ami-a4dc46db", os.AMD64Arch, ec2os.AmazonLinuxOS) // ubuntu-16-04-4.4
	e2e.Run(t, s, e2eParams...)
}

// SetupSuite
func (v *ec2VMContainerizedSuite) SetupSuite() {
	v.BaseSuite.SetupSuite()

	v.Env().RemoteHost.MustExecute("sudo apt install -y apache2-utils")

	// prefetch docker image locally
	v.Env().RemoteHost.MustExecute("docker run curlimages/curl --version")
	v.Env().RemoteHost.MustExecute("docker run devth/alpine-bench -V")
	v.Env().RemoteHost.MustExecute("docker run makocchi/alpine-dig dig")
}

// BeforeTest will be called before each test
func (v *ec2VMContainerizedSuite) BeforeTest(suiteName, testName string) {
	v.BaseSuite.BeforeTest(suiteName, testName)
	v.beforeTest(suiteName, testName)
}

func (v *ec2VMContainerizedSuite) beforeTest(suiteName, testName string) {
	// default is to reset the current state of the fakeintake aggregators
	if !v.BaseSuite.IsDevMode() {
		v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// testFakeIntakeNPM
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
func (v *ec2VMContainerizedSuite) testFakeIntakeNPM() {
	t := v.T()

	targetHostnameNetID := ""
	// looking for 1 host to send CollectorConnections payload to the fakeintake
	v.EventuallyWithT(func(c *assert.CollectT) {
		hostnameNetID, err := v.Env().FakeIntake.Client().GetConnectionsNames()
		assert.NoError(c, err, "GetConnectionsNames() errors")
		if !assert.NotEmpty(c, hostnameNetID, "no connections yet") {
			return
		}
		targetHostnameNetID = hostnameNetID[0]

		t.Logf("hostname+networkID %v seen connections", hostnameNetID)
	}, 60*time.Second, time.Second, "no connections received")

	// looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
	v.EventuallyWithT(func(c *assert.CollectT) {
		cnx, err := v.Env().FakeIntake.Client().GetConnections()
		assert.NoError(t, err)

		if !assert.Greater(c, len(cnx.GetPayloadsByName(targetHostnameNetID)), 2, "not enough payloads") {
			return
		}
		var payloadsTimestamps []time.Time
		for _, cc := range cnx.GetPayloadsByName(targetHostnameNetID) {
			payloadsTimestamps = append(payloadsTimestamps, cc.GetCollectedTime())
		}
		dt := payloadsTimestamps[2].Sub(payloadsTimestamps[1]).Seconds()
		t.Logf("hostname+networkID %v diff time %f seconds", targetHostnameNetID, dt)

		// we want the test fail now, not retrying on the next payloads
		assert.Greater(t, 0.5, math.Abs(dt-30), "delta between collection is higher than 500ms")
	}, 90*time.Second, time.Second, "not enough connections received")
}

// TestFakeIntakeNPM Validate the agent can communicate with the (fake) backend and send connections every 30 seconds
// 2 tests generate the request on the host and on docker
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
func (v *ec2VMContainerizedSuite) TestFakeIntakeNPM() {
	vt := v.T()
	vt.Run("host", func(t *testing.T) {
		v.BaseSuite.SetT(t)
		v.beforeTest("TestEC2VMContainerizedSuite", t.Name()) // workaround as suite doesn't call BeforeTest before each sub tests

		// generate a connection
		v.Env().RemoteHost.MustExecute("curl http://www.datadoghq.com")

		v.testFakeIntakeNPM()
	})
	vt.Run("docker", func(t *testing.T) {
		v.BaseSuite.SetT(t)
		v.beforeTest("TestEC2VMContainerizedSuite", t.Name()) // workaround as suite doesn't call BeforeTest before each sub tests

		// generate a connection
		v.Env().RemoteHost.MustExecute("docker run curlimages/curl curl http://www.datadoghq.com")

		v.testFakeIntakeNPM()
	})
}

// TestFakeIntakeNPM_600cnx_bucket Validate the agent can communicate with the (fake) backend and send connections
// every 30 seconds with a maximum of 600 connections per payloads, if more another payload will follow.
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for n payloads and check if the last 2 have a maximum span of 100ms
func (v *ec2VMContainerizedSuite) testFakeIntakeNPM_600cnx_bucket() {
	t := v.T()

	targetHostnameNetID := ""
	// looking for 1 host to send CollectorConnections payload to the fakeintake
	v.EventuallyWithT(func(c *assert.CollectT) {
		hostnameNetID, err := v.Env().FakeIntake.Client().GetConnectionsNames()
		assert.NoError(c, err, "GetConnectionsNames() errors")
		if !assert.NotEmpty(c, hostnameNetID, "no connections yet") {
			return
		}
		targetHostnameNetID = hostnameNetID[0]

		t.Logf("hostname+networkID %v seen connections", hostnameNetID)
	}, 60*time.Second, time.Second, "no connections received")

	// looking for x payloads (with max 600 connections) and check if the last 2 have a max span of 100ms
	v.EventuallyWithT(func(c *assert.CollectT) {
		cnx, err := v.Env().FakeIntake.Client().GetConnections()
		assert.NoError(t, err)

		if !assert.Greater(c, len(cnx.GetPayloadsByName(targetHostnameNetID)), 2, "not enough payloads") {
			return
		}

		/*
			cnx.ForeachHostnameConnections(func(cnx *aggregator.Connections, hostname string) {
				assert.LessOrEqualf(t, len(cnx.Connections), 600, "too many payloads")
			})
		*/

		hostPayloads := cnx.GetPayloadsByName(targetHostnameNetID)
		lenHostPayloads := len(hostPayloads)
		if !assert.Equalf(c, len(hostPayloads[lenHostPayloads-2].Connections), 600, "can't found enough connections 600+") {
			return
		}

		cnx600PayloadTime := hostPayloads[lenHostPayloads-2].GetCollectedTime()
		latestPayloadTime := hostPayloads[lenHostPayloads-1].GetCollectedTime()

		dt := latestPayloadTime.Sub(cnx600PayloadTime).Seconds()
		t.Logf("hostname+networkID %v diff time %f seconds", targetHostnameNetID, dt)

		assert.Greater(t, 0.1, dt, "delta between collection is higher than 100ms")
	}, 90*time.Second, time.Second, "not enough connections received")
}

// TestFakeIntakeNPM_600cnx_bucket Validate the agent can communicate with the (fake) backend and send connections
// every 30 seconds with a maximum of 600 connections per payloads, if more another payload will follow.
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for n payloads and check if the last 2 have a maximum span of 100ms
func (v *ec2VMContainerizedSuite) TestFakeIntakeNPM_600cnx_bucket() {
	vt := v.T()
	vt.Run("host", func(t *testing.T) {
		v.BaseSuite.SetT(t)
		v.beforeTest("TestEC2VMContainerizedSuite", t.Name()) // workaround as suite doesn't call BeforeTest before each sub tests

		// generate connections
		v.Env().RemoteHost.MustExecute("ab -n 600 -c 600 http://www.datadoghq.com/")

		v.testFakeIntakeNPM_600cnx_bucket()
	})
	vt.Run("docker", func(t *testing.T) {
		v.BaseSuite.SetT(t)
		v.beforeTest("TestEC2VMContainerizedSuite", t.Name()) // workaround as suite doesn't call BeforeTest before each sub tests

		// generate connections
		v.Env().RemoteHost.MustExecute("docker run devth/alpine-bench -n 600 -c 600 http://www.datadoghq.com/")

		v.testFakeIntakeNPM_600cnx_bucket()
	})
}

// TestFakeIntakeNPM_TCP_UDP_DNS validate we received tcp, udp, and DNS connections
// with some basic checks, like IPs/Ports present, DNS query has been captured, ...
func (v *ec2VMContainerizedSuite) testFakeIntakeNPM_TCP_UDP_DNS() {
	t := v.T()

	v.EventuallyWithT(func(c *assert.CollectT) {

		cnx, err := v.Env().FakeIntake.Client().GetConnections()
		assert.NoError(c, err, "GetConnections() errors")
		if !assert.NotEmpty(c, cnx.GetNames(), "no connections yet") {
			return
		}

		foundDNS := false
		cnx.ForeachConnection(func(c *agentmodel.Connection, cc *agentmodel.CollectorConnections, hostname string) {
			if len(c.DnsStatsByDomainOffsetByQueryType) > 0 {
				foundDNS = true
			}
		})
		if !assert.True(c, foundDNS, "DNS not found") {
			return
		}

		type countCnx struct {
			hit int
			TCP int
			UDP int
		}
		countConnections := make(map[string]*countCnx)

		helperCleanup(t)
		cnx.ForeachConnection(func(c *agentmodel.Connection, cc *agentmodel.CollectorConnections, hostname string) {
			var count *countCnx
			var found bool
			if count, found = countConnections[hostname]; !found {
				countConnections[hostname] = &countCnx{}
				count = countConnections[hostname]
			}
			count.hit++

			switch c.Type {
			case agentmodel.ConnectionType_tcp:
				count.TCP++
			case agentmodel.ConnectionType_udp:
				count.UDP++
			}
			validateConnection(t, c, cc, hostname)
		})

		totalConnections := countCnx{}
		for host, count := range countConnections {
			t.Logf("connections %d tcp %d udp %d received by host/netID %s", count.hit, count.TCP, count.UDP, host)
			totalConnections.hit += count.hit
			totalConnections.TCP += count.TCP
			totalConnections.UDP += count.UDP
		}
		t.Logf("sum connections %d tcp %d udp %d", totalConnections.hit, totalConnections.TCP, totalConnections.UDP)
	}, 60*time.Second, time.Second)
}

// TestFakeIntakeNPM_TCP_UDP_DNS validate we received tcp, udp, and DNS connections
// with some basic checks, like IPs/Ports present, DNS query has been captured, ...
func (v *ec2VMContainerizedSuite) TestFakeIntakeNPM_TCP_UDP_DNS() {
	vt := v.T()
	vt.Run("host", func(t *testing.T) {
		v.BaseSuite.SetT(t)
		v.beforeTest("TestEC2VMContainerizedSuite", t.Name()) // workaround as suite doesn't call BeforeTest before each sub tests

		// generate connections
		v.Env().RemoteHost.MustExecute("curl http://www.datadoghq.com")
		v.Env().RemoteHost.MustExecute("dig @8.8.8.8 www.google.ch")

		v.testFakeIntakeNPM_TCP_UDP_DNS()
	})
	vt.Run("docker", func(t *testing.T) {
		v.BaseSuite.SetT(t)
		v.beforeTest("TestEC2VMContainerizedSuite", t.Name()) // workaround as suite doesn't call BeforeTest before each sub tests

		// generate connections
		v.Env().RemoteHost.MustExecute("docker run curlimages/curl curl http://www.datadoghq.com")
		v.Env().RemoteHost.MustExecute("docker run makocchi/alpine-dig dig @8.8.8.8 www.google.ch")

		v.testFakeIntakeNPM_TCP_UDP_DNS()
	})
}

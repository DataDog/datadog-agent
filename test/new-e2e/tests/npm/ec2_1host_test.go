// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

type hostHttpbinEnv struct {
	environments.Host
	// Extra Components
	HTTPBinHost *components.RemoteHost
}
type ec2VMSuite struct {
	e2e.BaseSuite[hostHttpbinEnv]
}

func hostDockerHttpbinEnvProvisioner() e2e.PulumiEnvRunFunc[hostHttpbinEnv] {
	return func(ctx *pulumi.Context, env *hostHttpbinEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}
		env.Host.AwsEnvironment = &awsEnv

		opts := []awshost.ProvisionerOption{
			awshost.WithAgentOptions(agentparams.WithSystemProbeConfig(systemProbeConfigNPM)),
		}
		params := awshost.GetProvisionerParams(opts...)
		awshost.Run(ctx, &env.Host, params)

		vmName := "httpbinvm"

		nginxHost, err := ec2.NewVM(awsEnv, vmName)
		if err != nil {
			return err
		}
		err = nginxHost.Export(ctx, &env.HTTPBinHost.HostOutput)
		if err != nil {
			return err
		}

		// install docker.io
		manager, _, err := docker.NewManager(*awsEnv.CommonEnvironment, nginxHost, true)
		if err != nil {
			return err
		}

		composeContents := []docker.ComposeInlineManifest{dockerHTTPBinCompose()}
		_, err = manager.ComposeStrUp("httpbin", composeContents, pulumi.StringMap{})
		if err != nil {
			return err
		}

		return nil
	}
}

// TestEC2VMSuite will validate running the agent on a single EC2 VM
func TestEC2VMSuite(t *testing.T) {
	s := &ec2VMSuite{}

	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(e2e.NewTypedPulumiProvisioner("hostHttpbin", hostDockerHttpbinEnvProvisioner(), nil))}

	// Source of our kitchen CI images test/kitchen/platforms.json
	// Other VM image can be used, our kitchen CI images test/kitchen/platforms.json
	// ec2params.WithImageName("ami-a4dc46db", os.AMD64Arch, ec2os.AmazonLinuxOS) // ubuntu-16-04-4.4
	e2e.Run(t, s, e2eParams...)
}

func (v *ec2VMSuite) SetupSuite() {
	v.BaseSuite.SetupSuite()

	v.Env().RemoteHost.MustExecute("sudo apt install -y apache2-utils")
}

// BeforeTest will be called before each test
func (v *ec2VMSuite) BeforeTest(suiteName, testName string) {
	v.BaseSuite.BeforeTest(suiteName, testName)
	v.beforeTest(suiteName, testName)
}

func (v *ec2VMSuite) beforeTest(suiteName, testName string) {
	// default is to reset the current state of the fakeintake aggregators
	if !v.BaseSuite.IsDevMode() {
		v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// TestFakeIntakeNPM Validate the agent can communicate with the (fake) backend and send connections every 30 seconds
// 2 tests generate the request on the host and on docker
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
func (v *ec2VMSuite) TestFakeIntakeNPM() {
	vt := v.T()
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"
	vt.Run("host_requests", func(t *testing.T) {
		v.BaseSuite.SetT(t)
		v.beforeTest("TestEC2VMSuite", t.Name()) // workaround as suite doesn't call BeforeTest before each sub tests

		// generate a connection
		v.Env().RemoteHost.MustExecute("curl " + testURL)

		test1HostFakeIntakeNPM(&v.BaseSuite, v.Env().FakeIntake)
	})
	vt.Run("docker_requests", func(t *testing.T) {
		v.BaseSuite.SetT(t)
		v.beforeTest("TestEC2VMSuite", t.Name()) // workaround as suite doesn't call BeforeTest before each sub tests

		// generate a connection
		v.Env().RemoteHost.MustExecute("docker run curlimages/curl curl " + testURL)

		test1HostFakeIntakeNPM(&v.BaseSuite, v.Env().FakeIntake)
	})
}

// TestFakeIntakeNPM_600cnx_bucket Validate the agent can communicate with the (fake) backend and send connections
// every 30 seconds with a maximum of 600 connections per payloads, if more another payload will follow.
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for n payloads and check if the last 2 have a maximum span of 100ms
func (v *ec2VMSuite) TestFakeIntakeNPM_600cnx_bucket() {
	vt := v.T()
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"
	vt.Run("host_requests", func(t *testing.T) {
		v.BaseSuite.SetT(t)
		v.beforeTest("TestEC2VMSuite", t.Name()) // workaround as suite doesn't call BeforeTest before each sub tests

		// generate connections
		v.Env().RemoteHost.MustExecute("ab -n 600 -c 600 " + testURL)

		test1HostFakeIntakeNPM600cnxBucket(&v.BaseSuite, v.Env().FakeIntake)
	})
	vt.Run("docker_requests", func(t *testing.T) {
		v.BaseSuite.SetT(t)
		v.beforeTest("TestEC2VMSuite", t.Name()) // workaround as suite doesn't call BeforeTest before each sub tests

		// generate connections
		v.Env().RemoteHost.MustExecute("docker run devth/alpine-bench -n 600 -c 600 " + testURL)

		test1HostFakeIntakeNPM600cnxBucket(&v.BaseSuite, v.Env().FakeIntake)
	})
}

// TestFakeIntakeNPM_TCP_UDP_DNS validate we received tcp, udp, and DNS connections
// with some basic checks, like IPs/Ports present, DNS query has been captured, ...
func (v *ec2VMSuite) TestFakeIntakeNPM_TCP_UDP_DNS() {
	vt := v.T()
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"
	vt.Run("host_requests", func(t *testing.T) {
		v.BaseSuite.SetT(t)
		v.beforeTest("TestEC2VMSuite", t.Name()) // workaround as suite doesn't call BeforeTest before each sub tests

		// generate connections
		v.Env().RemoteHost.MustExecute("curl " + testURL)
		v.Env().RemoteHost.MustExecute("dig @8.8.8.8 www.google.ch")

		test1HostFakeIntakeNPMTCPUDPDNS(&v.BaseSuite, v.Env().FakeIntake)
	})
	vt.Run("docker_requests", func(t *testing.T) {
		v.BaseSuite.SetT(t)
		v.beforeTest("TestEC2VMSuite", t.Name()) // workaround as suite doesn't call BeforeTest before each sub tests

		// generate connections
		v.Env().RemoteHost.MustExecute("docker run curlimages/curl curl " + testURL)
		v.Env().RemoteHost.MustExecute("docker run makocchi/alpine-dig dig @8.8.8.8 www.google.ch")

		test1HostFakeIntakeNPMTCPUDPDNS(&v.BaseSuite, v.Env().FakeIntake)
	})
}

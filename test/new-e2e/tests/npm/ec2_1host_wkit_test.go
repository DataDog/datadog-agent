// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

const (
	TestDomain   = "datadogqalab.com"
	TestUser     = "TestUser"
	TestPassword = "Test1234#"
)

type hostWindowsHttpbinEnv struct {
	environments.Host
	// Extra Components
	HTTPBinHost *components.RemoteHost
}

type ec2VMWKitSuite struct {
	e2e.BaseSuite[hostWindowsHttpbinEnv]
}

func hostHttpbinEnvProvisioner(opt ...awshost.ProvisionerOption) e2e.PulumiEnvRunFunc[hostWindowsHttpbinEnv] {
	return func(ctx *pulumi.Context, env *hostWindowsHttpbinEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}
		env.Host.AwsEnvironment = &awsEnv

		opts := []awshost.ProvisionerOption{
			awshost.WithAgentOptions(agentparams.WithSystemProbeConfig(systemProbeConfigNPM)),
		}
		if len(opt) > 0 {
			opts = append(opts, opt...)
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

		return nil
	}
}

// TestEC2VMWKitSuite will validate running the agent on a single EC2 VM
func TestEC2VMWKitSuite(t *testing.T) {
	s := &ec2VMWKitSuite{}

	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(e2e.NewTypedPulumiProvisioner("hostHttpbin", hostHttpbinEnvProvisioner(awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault))), nil))}

	e2e.Run(t, s, e2eParams...)
}

// BeforeTest will be called before each test
func (v *ec2VMWKitSuite) BeforeTest(suiteName, testName string) {
	v.BaseSuite.BeforeTest(suiteName, testName)

	// default is to reset the current state of the fakeintake aggregators
	if !v.BaseSuite.IsDevMode() {
		v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// TestFakeIntakeNPM_HostRequests Validate the agent can communicate with the (fake) backend and send connections every 30 seconds
// 2 tests generate the request on the host and on docker
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for 3 payloads and check if the last 2 have a span of 30s +/- 500ms
func (v *ec2VMWKitSuite) TestFakeIntakeNPM_HostRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	v.Env().RemoteHost.MustExecute("$result = Invoke-WebRequest -UseBasicParsing -Uri " + testURL)

	test1HostFakeIntakeNPM(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM600cnxBucket_HostRequests Validate the agent can communicate with the (fake) backend and send connections
// every 30 seconds with a maximum of 600 connections per payloads, if more another payload will follow.
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for n payloads and check if the last 2 have a maximum span of 100ms
func (v *ec2VMWKitSuite) TestFakeIntakeNPM600cnxBucket_HostRequests() {
	//testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	v.T().Skip("TODO need to find a way to generate 600 cnx from windows powershell")
	// generate connections
	//v.Env().RemoteHost.MustExecute("ab -n 600 -c 600 " + testURL)

	test1HostFakeIntakeNPM600cnxBucket(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM_TCP_UDP_DNS_HostRequests validate we received tcp, udp, and DNS connections
// with some basic checks, like IPs/Ports present, DNS query has been captured, ...
func (v *ec2VMWKitSuite) TestFakeIntakeNPM_TCP_UDP_DNS_HostRequests() {
	testURL := "http://" + v.Env().HTTPBinHost.Address + "/"

	// generate connections
	v.Env().RemoteHost.MustExecute("$result = Invoke-WebRequest -UseBasicParsing -Uri " + testURL)
	v.Env().RemoteHost.MustExecute("Resolve-DnsName -Name www.google.ch -Server 8.8.8.8")

	test1HostFakeIntakeNPMTCPUDPDNS(&v.BaseSuite, v.Env().FakeIntake)
}

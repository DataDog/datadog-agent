// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	tifEcs "github.com/DataDog/test-infra-definitions/scenarios/aws/ecs"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	envecs "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/ecs"

	npmtools "github.com/DataDog/test-infra-definitions/components/datadog/apps/npm-tools"
	"github.com/DataDog/test-infra-definitions/components/datadog/ecsagentparams"
	"github.com/DataDog/test-infra-definitions/components/docker"
	ecsComp "github.com/DataDog/test-infra-definitions/components/ecs"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"

	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

type ecsHttpbinEnv struct {
	environments.ECS

	// Extra Components
	HTTPBinHost *components.RemoteHost
}

type ecsVMSuite struct {
	e2e.BaseSuite[ecsHttpbinEnv]
}

func ecsHttpbinEnvProvisioner() e2e.PulumiEnvRunFunc[ecsHttpbinEnv] {
	return func(ctx *pulumi.Context, env *ecsHttpbinEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

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
		manager, err := docker.NewManager(&awsEnv, nginxHost)
		if err != nil {
			return err
		}

		composeContents := []docker.ComposeInlineManifest{dockerHTTPBinCompose()}
		_, err = manager.ComposeStrUp("httpbin", composeContents, pulumi.StringMap{})
		if err != nil {
			return err
		}

		params := envecs.GetProvisionerParams(
			envecs.WithAwsEnv(&awsEnv),
			envecs.WithECSOptions(tifEcs.WithLinuxNodeGroup()),
			envecs.WithAgentOptions(ecsagentparams.WithAgentServiceEnvVariable("DD_SYSTEM_PROBE_NETWORK_ENABLED", "true")),
			envecs.WithWorkloadApp(func(e aws.Environment, clusterArn pulumi.StringInput) (*ecsComp.Workload, error) {
				testURL := "http://" + env.HTTPBinHost.Address + "/"
				return npmtools.EcsAppDefinition(e, clusterArn, testURL)
			}),
		)
		envecs.Run(ctx, &env.ECS, params)
		return nil
	}
}

// TestECSVMSuite will validate running the agent on a single EC2 VM
func TestECSVMSuite(t *testing.T) {
	t.Parallel()
	s := &ecsVMSuite{}
	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(e2e.NewTypedPulumiProvisioner("ecsHttpbin", ecsHttpbinEnvProvisioner(), nil))}

	e2e.Run(t, s, e2eParams...)
}

// BeforeTest will be called before each test
func (v *ecsVMSuite) BeforeTest(suiteName, testName string) {
	v.BaseSuite.BeforeTest(suiteName, testName)

	// default is to reset the current state of the fakeintake aggregators
	if !v.BaseSuite.IsDevMode() {
		v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// AfterTest will be called after each test
func (v *ecsVMSuite) AfterTest(suiteName, testName string) {
	test1HostFakeIntakeNPMDumpInfo(v.T(), v.Env().FakeIntake)

	v.BaseSuite.AfterTest(suiteName, testName)
}

// Test00FakeIntakeNPM Validate the agent can communicate with the (fake) backend and send connections every 30 seconds
// 2 tests generate the request on the host and on docker
//   - looking for 1 host to send CollectorConnections payload to the fakeintake
//   - looking for 5 payloads and check if the last 2 have a span of 30s +/- 500ms
//
// The test start by 00 to validate the agent/system-probe is up and running
// On ECS the agent is slow to start and this avoid flaky tests
func (v *ecsVMSuite) Test00FakeIntakeNPM() {
	flake.Mark(v.T())
	test1HostFakeIntakeNPM(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM_TCP_UDP_DNS_HostRequests validate we received tcp, udp, and DNS connections
// with some basic checks, like IPs/Ports present, DNS query has been captured, ...
func (v *ecsVMSuite) TestFakeIntakeNPM_TCP_UDP_DNS() {
	// mark the test flaky as somethimes 4/206 runs, it failed to retrieve DNS information
	flake.Mark(v.T())

	// deployed workload generate these connections every 20 seconds
	//v.Env().RemoteHost.MustExecute("curl " + testURL)
	//v.Env().RemoteHost.MustExecute("dig @8.8.8.8 www.google.ch")

	test1HostFakeIntakeNPMTCPUDPDNS(&v.BaseSuite, v.Env().FakeIntake)
}

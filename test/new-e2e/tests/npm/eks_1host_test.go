// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/common/config"
	npmtools "github.com/DataDog/test-infra-definitions/components/datadog/apps/npm-tools"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	"github.com/DataDog/test-infra-definitions/components/docker"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/eks"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	envkube "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
)

type eksHttpbinEnv struct {
	environments.Kubernetes

	// Extra Components
	HTTPBinHost *components.RemoteHost
}

type eksVMSuite struct {
	e2e.BaseSuite[eksHttpbinEnv]
}

func eksHttpbinEnvProvisioner(opts ...envkube.ProvisionerOption) e2e.PulumiEnvRunFunc[eksHttpbinEnv] {
	return func(ctx *pulumi.Context, env *eksHttpbinEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}
		vmName := "httpbinvm"
		httpbinHost, err := ec2.NewVM(awsEnv, vmName)
		if err != nil {
			return err
		}
		err = httpbinHost.Export(ctx, &env.HTTPBinHost.HostOutput)
		if err != nil {
			return err
		}

		// install docker.io
		manager, err := docker.NewManager(&awsEnv, httpbinHost)
		if err != nil {
			return err
		}

		composeContents := []docker.ComposeInlineManifest{dockerHTTPBinCompose()}
		_, err = manager.ComposeStrUp("httpbin", composeContents, pulumi.StringMap{})
		if err != nil {
			return err
		}

		npmToolsWorkload := func(_ config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
			// NPM tools Workload
			testURL := "http://" + env.HTTPBinHost.Address + "/"
			return npmtools.K8sAppDefinition(&awsEnv, kubeProvider, "npmtools", testURL)
		}

		provisionerOpts := []envkube.ProvisionerOption{
			envkube.WithAwsEnv(&awsEnv),
			envkube.WithEKSOptions(eks.WithLinuxNodeGroup()),
			envkube.WithAgentOptions(kubernetesagentparams.WithHelmValues(systemProbeConfigNPMHelmValues)),
			envkube.WithWorkloadApp(npmToolsWorkload),
		}
		provisionerOpts = append(provisionerOpts, opts...)

		params := envkube.GetProvisionerParams(
			provisionerOpts...,
		)
		envkube.EKSRunFunc(ctx, &env.Kubernetes, params)

		return nil
	}
}

// TestEKSVMSuite will validate running the agent
func TestEKSVMSuite(t *testing.T) {
	t.Parallel()

	s := &eksVMSuite{}
	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(e2e.NewTypedPulumiProvisioner("eksHttpbin", eksHttpbinEnvProvisioner(), nil))}

	e2e.Run(t, s, e2eParams...)
}

// BeforeTest will be called before each test
func (v *eksVMSuite) BeforeTest(suiteName, testName string) {
	v.BaseSuite.BeforeTest(suiteName, testName)
	// default is to reset the current state of the fakeintake aggregators
	if !v.BaseSuite.IsDevMode() {
		v.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// AfterTest will be called after each test
func (v *eksVMSuite) AfterTest(suiteName, testName string) {
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
func (v *eksVMSuite) Test00FakeIntakeNPM() {
	test1HostFakeIntakeNPM(&v.BaseSuite, v.Env().FakeIntake)
}

// TestFakeIntakeNPM_TCP_UDP_DNS_HostRequests validate we received tcp, udp, and DNS connections
// with some basic checks, like IPs/Ports present, DNS query has been captured, ...
func (v *eksVMSuite) TestFakeIntakeNPM_TCP_UDP_DNS() {
	// deployed workload generate these connections every 20 seconds
	//v.Env().RemoteHost.MustExecute("curl " + testURL)
	//v.Env().RemoteHost.MustExecute("dig @8.8.8.8 www.google.ch")

	test1HostFakeIntakeNPMTCPUDPDNS(&v.BaseSuite, v.Env().FakeIntake)
}

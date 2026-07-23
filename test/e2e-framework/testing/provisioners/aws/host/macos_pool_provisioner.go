// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package awshost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	rootcomponents "github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2/pool"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	e2eclient "github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
)

const macOSPoolProvisionerBaseID = "aws-ec2vm-macos-pool-"

// macOSPoolHostSSHUser is the AMI's default login user for every macOS flavor.
const macOSPoolHostSSHUser = "ec2-user"

// sshWaitTimeout/sshWaitRetryInterval bound how long ProvisionEnv waits for a freshly
// launched instance to accept SSH connections.
const (
	sshWaitTimeout       = 15 * time.Minute
	sshWaitRetryInterval = 15 * time.Second
)

// macOSPoolProvisioner provisions a macOS environments.Host directly through the AWS
// SDK and the resources/aws/ec2/pool package, bypassing Pulumi: it acquires a member of
// the shared macOS EC2 pool and waits for it to become SSH-reachable, instead of
// relying on a Pulumi remote.Host resource. FakeIntake and Agent, if requested, are
// likewise provisioned Pulumi-free (raw ECS Fargate calls and sequential SSH commands,
// respectively).
type macOSPoolProvisioner struct {
	id        string
	runParams *ec2.Params

	region     string
	profile    string
	instanceID string
	leaseToken string
	fakeIntake *macOSPoolFakeIntake
}

var _ provisioners.TypedProvisioner[environments.Host] = &macOSPoolProvisioner{}

// NewMacOSPoolProvisioner returns a Pulumi-free provisioner for a macOS
// environments.Host, drawing from the shared EC2 pool. runParams carries the resolved
// ec2.Params (instance options plus any requested Agent/FakeIntake configuration) for
// the caller's run.
func NewMacOSPoolProvisioner(name string, runParams *ec2.Params) provisioners.TypedProvisioner[environments.Host] {
	return &macOSPoolProvisioner{
		id:        macOSPoolProvisionerBaseID + name,
		runParams: runParams,
	}
}

// ID returns the ID of the provisioner.
func (p *macOSPoolProvisioner) ID() string {
	return p.id
}

// ProvisionEnv acquires an existing macOS pool instance and imports it into env.RemoteHost,
// then provisions FakeIntake and the Agent Pulumi-free if requested by p.runParams.
func (p *macOSPoolProvisioner) ProvisionEnv(ctx context.Context, _ string, logger io.Writer, env *environments.Host) (provisioners.RawResources, error) {
	// The Updater is never supported Pulumi-free.
	env.DisableUpdater()
	if !p.runParams.HasFakeIntake() {
		env.DisableFakeIntake()
	}
	if !p.runParams.HasAgent() {
		env.DisableAgent()
	}

	vmArgs, err := ec2.ResolveMacOSPoolArgs(p.runParams.InstanceOptions()...)
	if err != nil {
		return nil, err
	}

	cfg, err := pool.LoadLaunchConfigFromEnv()
	if err != nil {
		return nil, err
	}
	p.region = cfg.Region
	p.profile = cfg.Profile

	ec2Client, err := pool.NewEC2Client(ctx, cfg.Region, cfg.Profile)
	if err != nil {
		return nil, err
	}

	ownerID := macOSPoolOwnerID()

	acquired, err := pool.Acquire(ctx, cfg.Region, cfg.Profile, ec2Client, ownerID)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(logger, "reusing macOS pool instance %s\n", acquired.InstanceID)
	p.instanceID = acquired.InstanceID
	p.leaseToken = acquired.LeaseToken

	privateIP, err := pool.DescribeInstance(ctx, ec2Client, p.instanceID)
	if err != nil {
		return nil, err
	}

	hostOutput := remote.HostOutput{
		CloudProvider: rootcomponents.CloudProviderAWS,
		Address:       privateIP,
		Port:          22,
		Username:      macOSPoolHostSSHUser,
		OSFamily:      vmArgs.OSInfo.Family(),
		OSFlavor:      vmArgs.OSInfo.Flavor,
		OSVersion:     vmArgs.OSInfo.Version,
		Architecture:  vmArgs.OSInfo.Architecture,
	}

	sshHost, err := waitForSSH(ctx, logger, hostOutput)
	if err != nil {
		return nil, fmt.Errorf("macOS pool instance %s never became SSH-reachable: %w", p.instanceID, err)
	}

	if env.RemoteHost == nil {
		env.RemoteHost = &components.RemoteHost{}
	}
	env.RemoteHost.SetKey(p.id)

	marshalledHost, err := json.Marshal(hostOutput)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal macOS pool host output: %w", err)
	}
	resources := provisioners.RawResources{p.id: marshalledHost}

	var fiOutput *fakeintake.FakeintakeOutput
	if p.runParams.HasFakeIntake() {
		fiKey := p.id + "-fakeintake"

		var fiState *macOSPoolFakeIntake
		fiState, fiOutput, err = provisionMacOSPoolFakeIntake(ctx, logger, cfg.Region, cfg.Profile, cfg.EnvName, p.runParams.FakeIntakeOptions())
		p.fakeIntake = fiState
		if err != nil {
			return nil, fmt.Errorf("failed to provision macOS pool FakeIntake: %w", err)
		}

		marshalledFI, err := json.Marshal(fiOutput)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal macOS pool FakeIntake output: %w", err)
		}
		env.FakeIntakeOutput() // ensures env.FakeIntake is non-nil
		env.FakeIntake.SetKey(fiKey)
		resources[fiKey] = marshalledFI
	}

	if p.runParams.HasAgent() {
		agentKey := p.id + "-agent"

		apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve Agent API key for macOS pool provisioner: %w", err)
		}

		var fakeIntakeExtraConfig string
		if fiOutput != nil {
			fakeIntakeExtraConfig, err = macOSFakeIntakeExtraConfig(fiOutput)
			if err != nil {
				return nil, fmt.Errorf("failed to build FakeIntake agent config: %w", err)
			}
		}

		agentOutput, err := provisionMacOSPoolAgent(sshHost, hostOutput, apiKey, p.runParams.AgentOptions(), fakeIntakeExtraConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to provision macOS pool Agent: %w", err)
		}

		marshalledAgent, err := json.Marshal(agentOutput)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal macOS pool Agent output: %w", err)
		}
		env.AgentOutput() // ensures env.Agent is non-nil
		env.Agent.SetKey(agentKey)
		env.Agent.ClientOptions = p.runParams.AgentClientOptions()
		resources[agentKey] = marshalledAgent
	}

	return resources, nil
}

// waitForSSH polls client.NewHost until it succeeds or sshWaitTimeout elapses, returning
// the connected client so callers don't have to reconnect for follow-up work (e.g. the
// Agent install).
func waitForSSH(ctx context.Context, logger io.Writer, hostOutput remote.HostOutput) (*e2eclient.Host, error) {
	deadline := time.Now().Add(sshWaitTimeout)
	sshCtx := &sshWaitContext{logger: logger}

	for {
		host, err := e2eclient.NewHost(sshCtx, hostOutput)
		if err == nil {
			return host, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		select {
		case <-time.After(sshWaitRetryInterval):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// sshWaitContext adapts an io.Writer logger to client.Context for waitForSSH's use of
// client.NewHost.
type sshWaitContext struct {
	logger io.Writer
}

func (c *sshWaitContext) Logf(format string, args ...any) {
	fmt.Fprintf(c.logger, format+"\n", args...)
}

func (c *sshWaitContext) FailNow(format string, args ...any) {
	panic(fmt.Sprintf(format, args...))
}

func (c *sshWaitContext) SessionOutputDir() string {
	return ""
}

// macOSPoolOwnerID labels lease records with the CI pipeline that claimed them,
// falling back to a fixed label for local/standalone runs.
func macOSPoolOwnerID() string {
	if pipelineID := os.Getenv("CI_PIPELINE_ID"); pipelineID != "" {
		return pipelineID
	}
	return "local"
}

// Destroy tears down the FakeIntake ECS resources (if any were provisioned), resets the
// pool instance's root volume to its launch state, and releases its lease. It is a no-op
// on the pool instance if ProvisionEnv never successfully claimed one.
func (p *macOSPoolProvisioner) Destroy(ctx context.Context, _ string, logger io.Writer) error {
	if p.fakeIntake != nil {
		fmt.Fprintf(logger, "tearing down macOS pool FakeIntake ECS resources\n")
		if err := p.fakeIntake.destroy(ctx); err != nil {
			return fmt.Errorf("failed to destroy macOS pool FakeIntake: %w", err)
		}
		p.fakeIntake = nil
	}

	if p.instanceID == "" {
		return nil
	}

	ec2Client, err := pool.NewEC2Client(ctx, p.region, p.profile)
	if err != nil {
		return err
	}

	fmt.Fprintf(logger, "resetting macOS pool instance %s root volume before release\n", p.instanceID)
	if err := pool.ResetRootVolume(ctx, ec2Client, p.instanceID); err != nil {
		return fmt.Errorf("failed to reset root volume for macOS pool instance %s: %w", p.instanceID, err)
	}

	if err := pool.ReleaseInstance(ctx, p.region, p.profile, p.instanceID, p.leaseToken); err != nil {
		return fmt.Errorf("failed to release macOS pool instance %s: %w", p.instanceID, err)
	}

	p.instanceID = ""
	p.leaseToken = ""
	return nil
}

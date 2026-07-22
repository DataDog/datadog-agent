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
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2/pool"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
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
// relying on a Pulumi remote.Host resource.
type macOSPoolProvisioner struct {
	id   string
	opts []ec2.VMOption

	region     string
	profile    string
	instanceID string
	leaseToken string
}

var _ provisioners.TypedProvisioner[environments.Host] = &macOSPoolProvisioner{}

// NewMacOSPoolProvisioner returns a Pulumi-free provisioner for a macOS
// environments.Host, drawing from the shared EC2 pool. opts are the same VMOptions
// passed to ec2.WithEC2InstanceOptions.
func NewMacOSPoolProvisioner(name string, opts ...ec2.VMOption) provisioners.TypedProvisioner[environments.Host] {
	return &macOSPoolProvisioner{
		id:   macOSPoolProvisionerBaseID + name,
		opts: opts,
	}
}

// ID returns the ID of the provisioner.
func (p *macOSPoolProvisioner) ID() string {
	return p.id
}

// ProvisionEnv acquires an existing macOS pool instance and imports it into env.RemoteHost.
func (p *macOSPoolProvisioner) ProvisionEnv(ctx context.Context, _ string, logger io.Writer, env *environments.Host) (provisioners.RawResources, error) {
	// This provisioner only ever imports RemoteHost, so Agent/FakeIntake/Updater must
	// be explicitly disabled.
	env.DisableAgent()
	env.DisableFakeIntake()
	env.DisableUpdater()

	vmArgs, err := ec2.ResolveMacOSPoolArgs(p.opts...)
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

	if err := waitForSSH(ctx, logger, hostOutput); err != nil {
		return nil, fmt.Errorf("macOS pool instance %s never became SSH-reachable: %w", p.instanceID, err)
	}

	if env.RemoteHost == nil {
		env.RemoteHost = &components.RemoteHost{}
	}
	env.RemoteHost.SetKey(p.id)

	marshalled, err := json.Marshal(hostOutput)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal macOS pool host output: %w", err)
	}

	return provisioners.RawResources{p.id: marshalled}, nil
}

// waitForSSH polls client.NewHost until it succeeds or sshWaitTimeout elapses.
func waitForSSH(ctx context.Context, logger io.Writer, hostOutput remote.HostOutput) error {
	deadline := time.Now().Add(sshWaitTimeout)
	sshCtx := &sshWaitContext{logger: logger}

	for {
		_, err := e2eclient.NewHost(sshCtx, hostOutput)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		select {
		case <-time.After(sshWaitRetryInterval):
		case <-ctx.Done():
			return ctx.Err()
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

// Destroy resets the pool instance's root volume to its launch state and releases its
// lease. It is a no-op if ProvisionEnv never successfully claimed an instance.
func (p *macOSPoolProvisioner) Destroy(ctx context.Context, _ string, logger io.Writer) error {
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

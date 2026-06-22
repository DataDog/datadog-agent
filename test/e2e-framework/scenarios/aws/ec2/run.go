// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/updater"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	fakeintakescenario "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Run deploys an environment given a pulumi.Context.
// It accepts HostOutputs interface, which is implemented by both:
// - outputs.Host (lightweight, for scenarios without test dependencies)
// - environments.Host (full-featured, for test provisioners)
func Run(ctx *pulumi.Context, awsEnv aws.Environment, env outputs.HostOutputs, params *Params) error {

	host, err := NewVM(awsEnv, params.Name, params.instanceOptions...)
	if err != nil {
		return err
	}
	err = host.Export(ctx, env.RemoteHostOutput())
	if err != nil {
		return err
	}

	if params.installDocker {
		dockerManager, err := docker.NewAWSManager(&awsEnv, host)

		if err != nil {
			return err
		}
		if params.agentOptions != nil {
			// Agent install needs to be serial with the docker
			// install because they both use the apt lock, and
			// can cause each others' installs to fail if run
			// at the same time.
			params.agentOptions = append(params.agentOptions,
				agentparams.WithPulumiResourceOptions(
					utils.PulumiDependsOn(dockerManager)))
		}
	}

	// Create FakeIntake if required
	if params.fakeintakeOptions != nil {
		fakeIntake, err := fakeintakescenario.NewECSFargateInstance(awsEnv, params.Name, params.fakeintakeOptions...)

		if err != nil {
			return err
		}
		err = fakeIntake.Export(ctx, env.FakeIntakeOutput())
		if err != nil {
			return err
		}

		// Normally if FakeIntake is enabled, Agent is enabled, but just in case.
		// WithFakeintake automatically wires RC — no separate RC option needed.
		if params.agentOptions != nil {
			params.agentOptions = append([]agentparams.Option{agentparams.WithFakeintake(fakeIntake)}, params.agentOptions...)
		}
	} else {
		// Mark FakeIntake as not provisioned
		env.DisableFakeIntake()
		// When not using fakeintake, apply the configured Datadog site so the
		// agent reports to the correct backend (e.g. datad0g.com for dev stacks).
		if params.agentOptions != nil {
			if site := awsEnv.Site(); site != "" {
				params.agentOptions = append(params.agentOptions, func(p *agentparams.Params) error {
					p.ExtraAgentConfig = append(p.ExtraAgentConfig, pulumi.String("site: "+site))
					return nil
				})
			}
		}
	}
	// Tags are agent metadata and apply regardless of fakeintake.
	if params.agentOptions != nil {
		if tags := awsEnv.Tags(); len(tags) > 0 {
			params.agentOptions = append(params.agentOptions, agentparams.WithTags(tags))
		}
	}
	if !params.installUpdater {
		// Mark Updater as not provisioned
		env.DisableUpdater()
	}

	// Create Agent if required
	if params.installUpdater && params.agentOptions != nil {
		updater, err := updater.NewHostUpdater(&awsEnv, host, params.agentOptions...)
		if err != nil {
			return err
		}

		err = updater.Export(ctx, env.UpdaterOutput())
		if err != nil {
			return err
		}
		// todo: add agent once updater installs agent on bootstrap
		env.DisableAgent()
	} else if params.agentOptions != nil {
		agentOptions := append(params.agentOptions, agentparams.WithTags([]string{fmt.Sprintf("stackid:%s", ctx.Stack())}))
		agent, err := agent.NewHostAgent(&awsEnv, host, agentOptions...)
		if err != nil {
			return err
		}

		err = agent.Export(ctx, env.AgentOutput())
		if err != nil {
			return err
		}
		env.SetAgentClientOptions(params.agentClientOptions...)
	} else {
		// Mark Agent as not provisioned
		env.DisableAgent()
	}

	return nil
}

// VMRun is the entry point for the scenario when run via pulumi.
// It uses outputs.Host which is lightweight and doesn't pull in test dependencies.
func VMRun(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env := outputs.NewHost()

	return Run(ctx, awsEnv, env, ParamsFromEnvironment(awsEnv))
}

// VMRunWithParams is like VMRun but merges the provided params on top of
// ParamsFromEnvironment. The override wins on nil/disabled decisions; its
// options are appended to the environment-derived base options otherwise.
func VMRunWithParams(ctx *pulumi.Context, override *Params) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env := outputs.NewHost()

	return Run(ctx, awsEnv, env, mergeParams(ParamsFromEnvironment(awsEnv), override))
}

// mergeParams applies override on top of base. For slice fields, nil in override
// means "disabled" (result is nil); otherwise override's entries are appended to
// base's. Boolean fields are OR-ed. Name uses override when it differs from the
// default.
func mergeParams(base, override *Params) *Params {
	merged := &Params{
		Name:           base.Name,
		installDocker:  base.installDocker || override.installDocker,
		installUpdater: base.installUpdater || override.installUpdater,
	}

	if override.Name != "" && override.Name != defaultVMName {
		merged.Name = override.Name
	}

	if override.instanceOptions == nil {
		merged.instanceOptions = base.instanceOptions
	} else {
		merged.instanceOptions = append(base.instanceOptions, override.instanceOptions...)
	}

	if override.agentOptions == nil {
		merged.agentOptions = nil
	} else if len(override.agentOptions) == 0 {
		merged.agentOptions = base.agentOptions
	} else {
		merged.agentOptions = append(base.agentOptions, override.agentOptions...)
	}

	if override.agentClientOptions == nil {
		merged.agentClientOptions = base.agentClientOptions
	} else {
		merged.agentClientOptions = append(base.agentClientOptions, override.agentClientOptions...)
	}

	if override.fakeintakeOptions == nil {
		merged.fakeintakeOptions = nil
	} else if len(override.fakeintakeOptions) == 0 {
		merged.fakeintakeOptions = base.fakeintakeOptions
	} else {
		merged.fakeintakeOptions = append(base.fakeintakeOptions, override.fakeintakeOptions...)
	}

	return merged
}

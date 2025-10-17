package computerun

import (
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/resources/gcp"
	"github.com/DataDog/test-infra-definitions/scenarios/gcp/compute"
	"github.com/DataDog/test-infra-definitions/scenarios/gcp/fakeintake"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func VMRun(ctx *pulumi.Context) error {
	env, err := gcp.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	vm, err := compute.NewVM(env, "gcp-vm")
	if err != nil {
		return err
	}
	if err := vm.Export(ctx, nil); err != nil {
		return err
	}

	if env.AgentDeploy() {
		agentOptions := []agentparams.Option{}
		if env.AgentUseFakeintake() {
			fakeintake, err := fakeintake.NewVMInstance(env)
			if err != nil {
				return err
			}
			if err := fakeintake.Export(ctx, nil); err != nil {
				return err
			}

			agentOptions = append(agentOptions, agentparams.WithFakeintake(fakeintake))
		}
		if env.AgentFlavor() != "" {
			agentOptions = append(agentOptions, agentparams.WithFlavor(env.AgentFlavor()))
		}
		if env.AgentConfigPath() != "" {
			configContent, err := env.CustomAgentConfig()
			if err != nil {
				return err
			}
			agentOptions = append(agentOptions, agentparams.WithAgentConfig(configContent))
		}
		_, err = agent.NewHostAgent(&env, vm, agentOptions...)
		return err
	}

	return nil
}

package localpodmanrun

import (
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/resources/local"
	localpodman "github.com/DataDog/test-infra-definitions/scenarios/local/podman"

	"github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func VMRun(ctx *pulumi.Context) error {
	env, err := local.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	vm, err := localpodman.NewVM(env, "podman-vm")
	if err != nil {
		return err
	}
	if err := vm.Export(ctx, nil); err != nil {
		return err
	}

	if env.AgentDeploy() {
		agentOptions := []agentparams.Option{}
		if env.AgentUseFakeintake() {
			fakeintake, err := fakeintake.NewLocalDockerFakeintake(&env, "fakeintake")
			if err != nil {
				return err
			}
			err = fakeintake.Export(ctx, nil)
			if err != nil {
				return err
			}
			agentOptions = append(agentOptions, agentparams.WithFakeintake(fakeintake))
		}
		if env.AgentFlavor() != "" {
			agentOptions = append(agentOptions, agentparams.WithFlavor(env.AgentFlavor()))
		}
		agentOptions = append(agentOptions, agentparams.WithHostname("localpodman-vm"))
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

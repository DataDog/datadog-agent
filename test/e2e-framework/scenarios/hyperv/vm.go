package kindvm

import (
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/resources/hyperv"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Run(ctx *pulumi.Context) error {
	env, err := hyperv.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	vm, err := hyperv.NewVM(env, hyperv.VMArgs{})
	if err != nil {
		return err
	}

	// From here forward to whatever you want with your VM, it's the same as any other VM
	_, err = agent.NewHostAgent(&env, vm)
	if err != nil {
		return err
	}

	return nil
}

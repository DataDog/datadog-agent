package fakeintake

import (
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/command"
	"github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/azure"
	"github.com/DataDog/test-infra-definitions/scenarios/azure/compute"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func NewVMInstance(e azure.Environment, option ...Option) (*fakeintake.Fakeintake, error) {
	params, paramsErr := NewParams(option...)
	if paramsErr != nil {
		return nil, paramsErr
	}

	return components.NewComponent(&e, "fakeintake", func(fi *fakeintake.Fakeintake) error {

		vm, err := compute.NewVM(e, "fakeintake", compute.WithOS(os.UbuntuDefault), compute.WithPulumiResourceOptions(pulumi.Parent(fi)))
		if err != nil {
			return err
		}
		manager, err := docker.NewManager(&e, vm, pulumi.Parent(vm))
		if err != nil {
			return err
		}
		cmdArgs := []string{}

		if params.DDDevForwarding {
			cmdArgs = append(cmdArgs, "--dddev-forward")
		}

		if params.RetentionPeriod != "" {
			cmdArgs = append(cmdArgs, "-retention-period="+params.RetentionPeriod)
		}

		if params.StoreStype != "" {
			cmdArgs = append(cmdArgs, "-store="+params.StoreStype)
		}

		_, err = vm.OS.Runner().Command("docker_run_fakeintake", &command.Args{
			Create: pulumi.Sprintf("docker run --restart unless-stopped --name fakeintake -d -p 80:80 -e DD_API_KEY=%s %s %s", e.AgentAPIKey(), params.ImageURL, cmdArgs),
			Delete: pulumi.String("docker stop fakeintake"),
		}, utils.PulumiDependsOn(manager), pulumi.DeleteBeforeReplace(true))
		if err != nil {
			return err
		}

		fi.Host = vm.Address
		fi.Scheme = pulumi.Sprintf("%s", "http")
		fi.Port = pulumi.Int(80).ToIntOutput()
		fi.URL = pulumi.Sprintf("%s://%s:%v", fi.Scheme, vm.Address, fi.Port)

		return nil
	})
}

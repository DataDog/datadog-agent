// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fakeintake

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/azure"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/azure/compute"
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

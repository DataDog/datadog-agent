// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kindvm

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/hyperv"

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

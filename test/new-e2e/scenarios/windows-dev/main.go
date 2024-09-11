// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main is the entrypoint for the windows dev setup
package main

import (
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		env, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		// replace XXXXXXXX with target custom AMI
		vm, err := ec2.NewVM(env, "vm", ec2.WithAMI("ami-07cc1bbe145f35b58", os.WindowsDefault, os.AMD64Arch))
		if err != nil {
			return err
		}
		if err := vm.Export(ctx, nil); err != nil {
			return err
		}

		return nil
	})
}

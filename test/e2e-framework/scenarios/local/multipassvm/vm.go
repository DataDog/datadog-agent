// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package localmultipassvm

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	componentsos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"
	localmultipass "github.com/DataDog/datadog-agent/test/e2e-framework/resources/local/vm/multipass"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// NewVM creates a multipass Ubuntu VM and returns it as a Host component.
func NewVM(e local.Environment, name string, params ...VMOption) (*remote.Host, error) {
	return components.NewComponent(&e, e.Namer.ResourceName(name), func(c *remote.Host) error {
		vmArgs, err := buildArgs(params...)
		if err != nil {
			return err
		}
		instanceName := e.Namer.ResourceName(e.Ctx().Stack(), name)
		address, user, port, err := localmultipass.NewInstance(e, instanceName, vmArgs, vmArgs.PulumiResourceOptions...)
		if err != nil {
			return err
		}

		conn, err := remote.NewConnection(
			address,
			user,
			remote.WithPort(port),
		)
		if err != nil {
			return err
		}

		return remote.InitHost(&e, conn.ToConnectionOutput(), componentsos.Ubuntu2204, user, pulumi.String("").ToStringOutput(), command.WaitForSuccessfulConnection, c)
	})
}

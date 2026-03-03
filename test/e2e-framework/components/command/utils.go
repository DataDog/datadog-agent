// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package command

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ReadyFunc func(Runner) (Command, error)

func WaitForCloudInit(runner Runner) (Command, error) {
	return runner.Command(
		"wait-cloud-init",
		&Args{
			// `sudo` is required for amazon linux
			Create: pulumi.String("cloud-init status --wait"),
			Sudo:   true,
		})
}

func WaitForSuccessfulConnection(runner Runner) (Command, error) {
	return runner.Command(
		"wait-successful-connection",
		&Args{
			// echo works in shell and powershell
			Create: pulumi.String("echo \"OK\""),
		})
}

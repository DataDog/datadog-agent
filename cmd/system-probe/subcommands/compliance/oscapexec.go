// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package compliance

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/compliance/cli"
)

// oscapExecCommand returns the 'compliance oscap-exec' command
func oscapExecCommand(_ *command.GlobalParams) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "oscap-exec <binary-path> [args...]",
		Short:  "Execute oscap-io with dropped capabilities (internal use only)",
		Long:   "Internal command that drops capabilities before executing oscap-io. This command should not be called directly by users.",
		Hidden: true,
		Args:   cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return cli.RunOscapExec(args)
		},
	}

	return cmd
}

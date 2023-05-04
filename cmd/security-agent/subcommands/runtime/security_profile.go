// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package runtime

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/cmd/security-agent/flags"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func securityProfileCommands(globalParams *command.GlobalParams) []*cobra.Command {
	securityProfileCmd := &cobra.Command{
		Use:   "security-profile",
		Short: "security profile commands",
	}

	securityProfileCmd.AddCommand(securityProfileShowCommands(globalParams)...)

	return []*cobra.Command{securityProfileCmd}
}

func securityProfileShowCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &activityDumpCliParams{
		GlobalParams: globalParams,
	}

	securityProfileShowCmd := &cobra.Command{
		Use:   "show",
		Short: "dump the content of a security-profile file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(showSecurityProfile,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}

	securityProfileShowCmd.Flags().StringVar(
		&cliParams.file,
		flags.Input,
		"",
		"path to the activity dump file",
	)

	return []*cobra.Command{securityProfileShowCmd}
}

func showSecurityProfile(log log.Component, config config.Component, activityDumpArgs *activityDumpCliParams) error {
	prof, err := profile.LoadProfileFromFile(activityDumpArgs.file)
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(prof, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(b))

	return nil
}

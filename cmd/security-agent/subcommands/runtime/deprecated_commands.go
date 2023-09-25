// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package runtime

import (
	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/cmd/security-agent/flags"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

// checkPoliciesCommands is deprecated
func checkPoliciesCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &checkPoliciesCliParams{
		GlobalParams: globalParams,
	}

	checkPoliciesCmd := &cobra.Command{
		Use:   "check-policies",
		Short: "check policies and return a report",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(checkPolicies,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "off", false)}),
				core.Bundle,
			)
		},
		Deprecated: "please use `security-agent runtime policy check` instead",
	}

	checkPoliciesCmd.Flags().StringVar(&cliParams.dir, flags.PoliciesDir, pkgconfig.DefaultRuntimePoliciesDir, "Path to policies directory")

	return []*cobra.Command{checkPoliciesCmd}
}

// reloadPoliciesCommands is deprecated
func reloadPoliciesCommands(globalParams *command.GlobalParams) []*cobra.Command {
	reloadPoliciesCmd := &cobra.Command{
		Use:   "reload",
		Short: "Reload policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(reloadRuntimePolicies,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
		Deprecated: "please use `security-agent runtime policy reload` instead",
	}
	return []*cobra.Command{reloadPoliciesCmd}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package privilegedhelperenabled implements the 'privileged-helper-enabled'
// subcommand, which systemd runs before each start of the rshell privileged
// helper so the helper only runs when the Agent configuration enables it.
package privilegedhelperenabled

import (
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// disabledExitCode is returned when the helper is not enabled. Any non-zero
// status stops systemd from starting the helper.
const disabledExitCode = 1

type disabledError struct{}

func (disabledError) Error() string { return par.RestrictedShellPrivilegedEnabled + " is false" }

func (disabledError) ExitCode() int { return disabledExitCode }

// Commands returns the 'privileged-helper-enabled' subcommand.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := &cobra.Command{
		Use:    "privileged-helper-enabled",
		Short:  "Exit with status 0 only if the rshell privileged helper is enabled",
		Hidden: true,
		Long: `Loads the Agent configuration the way the Private Action Runner does and
exits with status 0 if private_action_runner.restricted_shell.privileged.enabled
is true, and 1 otherwise. systemd runs it as dd-agent before starting the
privileged helper, so the helper does no work unless the setting is enabled.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(run,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath)),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true),
				}),
				// No secrets: the setting is a plain boolean, and resolving
				// secrets would run the secret backend for every activation.
				core.Bundle(),
			)
		},
	}
	return []*cobra.Command{cmd}
}

func run(logger log.Component, cfg config.Component) error {
	if !cfg.GetBool(par.RestrictedShellPrivilegedEnabled) {
		logger.Infof("%s is false; not starting the rshell privileged helper", par.RestrictedShellPrivilegedEnabled)
		return disabledError{}
	}
	return nil
}

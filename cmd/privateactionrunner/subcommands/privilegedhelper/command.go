// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package privilegedhelper implements the 'privileged-helper' subcommand,
// which systemd runs as root to start the rshell privileged helper only when
// the Agent configuration enables it.
package privilegedhelper

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

// helperParams locates the rshell helper, its socket and its policy.
type helperParams struct {
	helperPath string
	socketPath string
	policyPath string
}

// Commands returns the 'privileged-helper' subcommand.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	params := &helperParams{}
	cmd := &cobra.Command{
		Use:    "privileged-helper",
		Short:  "Start the rshell privileged helper if the configuration enables it",
		Hidden: true,
		Long: `Loads the Agent configuration the way the Private Action Runner does. If
private_action_runner.restricted_shell.privileged.enabled is false, or the kernel
cannot run the helper, it exits with status 0 without creating the socket.
Otherwise it creates the helper socket and replaces itself with the rshell
privileged helper, which serves it. systemd runs it as root.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(run,
				fx.Supply(params),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath)),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true),
				}),
				// No secrets: this runs as root, and resolving secrets would run
				// the secret backend command. The setting is a plain boolean.
				core.Bundle(),
			)
		},
	}
	cmd.Flags().StringVar(&params.helperPath, "helper", "", "path to the rshell binary")
	cmd.Flags().StringVar(&params.socketPath, "socket", par.RShellPrivilegedSocketDefault, "path of the socket to create for the helper")
	cmd.Flags().StringVar(&params.policyPath, "policy", "", "path to the root-provisioned helper policy")
	return []*cobra.Command{cmd}
}

func run(logger log.Component, cfg config.Component, params *helperParams) error {
	if !cfg.GetBool(par.RestrictedShellPrivilegedEnabled) {
		logger.Infof("%s is false; not starting the rshell privileged helper", par.RestrictedShellPrivilegedEnabled)
		return nil
	}
	return start(logger, params)
}

// start is swapped in tests, which cannot start the helper.
var start = startHelper

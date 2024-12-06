// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"github.com/spf13/cobra"
)

func apmCommands() *cobra.Command {
	ctlCmd := &cobra.Command{
		Use:     "apm [command]",
		Short:   "Interact with the APM auto-injector",
		GroupID: "apm",
	}
	ctlCmd.AddCommand(apmInstrumentCommand(), apmUninstrumentCommand())
	return ctlCmd
}

func apmInstrumentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instrument [all|host|docker]",
		Short: "Instrument APM auto-injection for a host or docker. Defaults to both.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("apm_instrument")
			if err != nil {
				return err
			}
			defer func() { i.Stop(err) }()
			if len(args) == 0 {
				args = []string{"not_set"}
			}
			i.span.SetTag("params.instrument", args[0])
			return i.InstrumentAPMInjector(i.ctx, args[0])
		},
	}
	return cmd
}

func apmUninstrumentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstrument [all|host|docker]",
		Short: "Uninstrument APM auto-injection for a host or docker. Defaults to both.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i, err := newInstallerCmd("apm_uninstrument")
			if err != nil {
				return err
			}
			defer func() { i.Stop(err) }()
			if len(args) == 0 {
				args = []string{"not_set"}
			}
			i.span.SetTag("params.instrument", args[0])
			return i.UninstrumentAPMInjector(i.ctx, args[0])
		},
	}
	return cmd
}

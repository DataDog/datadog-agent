// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/spf13/cobra"
)

func postinstCommand() *cobra.Command {
	cmd := &cobra.Command{
		Hidden:  true,
		Use:     "postinst <package> <caller:deb|rpm|installer>",
		Short:   "Run post-install scripts for a package",
		GroupID: "installer",
		Args:    cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i := newCmd("postinst")
			defer i.stop(err)
			return installer.PostInstall(i.ctx, args[0], args[1])
		},
	}
	return cmd
}

func prermCommand() *cobra.Command {
	var update bool
	cmd := &cobra.Command{
		Hidden:  true,
		Use:     "prerm <package> <caller:deb|rpm|installer> [--update]",
		Short:   "Run pre-rms scripts for a package",
		GroupID: "installer",
		Args:    cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i := newCmd("prerm")
			defer i.stop(err)

			if args[1] == "installer" && update {
				return errors.New("update flag is not supported for 'installer' caller; use other state transitions")
			}

			return installer.PreRemove(i.ctx, args[0], args[1], update)
		},
	}
	cmd.Flags().BoolVar(&update, "update", false, "Set during updates, don't set during removes")
	return cmd
}

func preStartExpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Hidden:  true,
		Use:     "pre-start-experiment <package>",
		Short:   "Run pre-start-experiment scripts for a package",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i := newCmd("pre-start-experiment")
			defer i.stop(err)

			return installer.PreStartExperiment(i.ctx, args[0])
		},
	}
	return cmd
}

func postStartExpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Hidden:  true,
		Use:     "post-start-experiment <package>",
		Short:   "Run post-start-experiment scripts for a package",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i := newCmd("post-start-experiment")
			defer i.stop(err)

			return installer.PostStartExperiment(i.ctx, args[0])
		},
	}
	return cmd
}

func preStopExpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Hidden:  true,
		Use:     "pre-stop-experiment <package>",
		Short:   "Run pre-stop-experiment scripts for a package",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i := newCmd("pre-stop-experiment")
			defer i.stop(err)

			return installer.PreStopExperiment(i.ctx, args[0])
		},
	}
	return cmd
}

func postStopExpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Hidden:  true,
		Use:     "post-stop-experiment <package>",
		Short:   "Run post-stop-experiment scripts for a package",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i := newCmd("post-stop-experiment")
			defer i.stop(err)

			return installer.PostStopExperiment(i.ctx, args[0])
		},
	}
	return cmd
}

func prePromoteExpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Hidden:  true,
		Use:     "pre-promote-experiment <package>",
		Short:   "Run pre-promote-experiment scripts for a package",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i := newCmd("pre-promote-experiment")
			defer i.stop(err)

			return installer.PrePromoteExperiment(i.ctx, args[0])
		},
	}
	return cmd
}

func postPromoteExpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Hidden:  true,
		Use:     "post-promote-experiment <package>",
		Short:   "Run post-promote-experiment scripts for a package",
		GroupID: "installer",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) (err error) {
			i := newCmd("post-promote-experiment")
			defer i.stop(err)

			return installer.PostPromoteExperiment(i.ctx, args[0])
		},
	}
	return cmd
}

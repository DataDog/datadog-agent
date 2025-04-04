// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

import (
	"errors"

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
			i, err := newInstallerCmd("postinst")
			if err != nil {
				return err
			}
			defer i.stop(err)
			return i.Postinst(i.ctx, args[0], args[1])
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
			i, err := newInstallerCmd("prerm")
			if err != nil {
				return err
			}
			defer i.stop(err)

			if args[1] == "installer" && update {
				return errors.New("update flag is not supported for 'installer' caller; use other state transitions")
			}

			return i.Prerm(i.ctx, args[0], args[1], update)
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
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			i, err := newInstallerCmd("pre-start-experiment")
			if err != nil {
				return err
			}
			defer i.stop(err)

			panic("TODO: not implemented")
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
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			i, err := newInstallerCmd("post-start-experiment")
			if err != nil {
				return err
			}
			defer i.stop(err)

			panic("TODO: not implemented")
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
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			i, err := newInstallerCmd("pre-stop-experiment")
			if err != nil {
				return err
			}
			defer i.stop(err)

			panic("TODO: not implemented")
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
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			i, err := newInstallerCmd("post-stop-experiment")
			if err != nil {
				return err
			}
			defer i.stop(err)

			panic("TODO: not implemented")
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
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			i, err := newInstallerCmd("pre-promote-experiment")
			if err != nil {
				return err
			}
			defer i.stop(err)

			panic("TODO: not implemented")
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
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			i, err := newInstallerCmd("post-promote-experiment")
			if err != nil {
				return err
			}
			defer i.stop(err)

			panic("TODO: not implemented")
		},
	}
	return cmd
}

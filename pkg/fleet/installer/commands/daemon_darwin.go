// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package commands

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
)

// platformCommands returns the commands the .pkg scripts use to manage launchd jobs.
//
// They exist so each job's launchd definition lives in exactly one place -- the embedded copy the
// Fleet install path also writes -- rather than being duplicated as plist XML inside a shell
// script, where it would silently drift from the definition the daemon itself maintains.
func platformCommands() []*cobra.Command {
	return []*cobra.Command{
		installStableJobsCommand(),
		uninstallDaemonCommand(),
	}
}

func installStableJobsCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "install-stable-jobs",
		Short:   "Installs and starts the agent, system-probe, Agent Data Plane and installer daemon launchd jobs",
		GroupID: "installer",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			i := newCmd("install_stable_jobs")
			defer func() { i.stop(err) }()
			return packages.InstallStableJobs(i.ctx)
		},
	}
}

func uninstallDaemonCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "uninstall-daemon",
		Short:   "Stops the installer daemon and removes its launchd job",
		GroupID: "installer",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			i := newCmd("uninstall_daemon")
			defer func() { i.stop(err) }()
			// Removal is best effort by design: a job that is already gone is the outcome the
			// caller wants, and an uninstall that failed here would leave the package half
			// removed for no gain.
			packages.RemoveDaemonJob(i.ctx)
			return nil
		},
	}
}

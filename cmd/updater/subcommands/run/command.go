// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package run implements 'updater run'.
package run

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/updater/command"
	"github.com/DataDog/datadog-agent/pkg/updater"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/spf13/cobra"
)

// Commands returns the run command
func Commands(global *command.GlobalParams) []*cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Runs the updater",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(global.Package, global.RepositoriesDir, global.RunPath, global.WatchProcesses)
		},
	}
	return []*cobra.Command{runCmd}
}

func run(pkg string, repositoriesPath string, runPath string, watchProcesses bool) error {
	orgConfig, err := updater.NewOrgConfig()
	if err != nil {
		return fmt.Errorf("could not create org config: %w", err)
	}
	u, err := updater.NewUpdater(orgConfig, pkg, repositoriesPath, runPath, watchProcesses)
	if err != nil {
		return fmt.Errorf("could not create updater: %w", err)
	}
	if watchProcesses {
		u.StartGC()
		defer u.StopGC()
		log.Debug("launched updater garbage collector")
	}

	api, err := updater.NewLocalAPI(u)
	if err != nil {
		return fmt.Errorf("could not create local API: %w", err)
	}
	return api.Serve()
}

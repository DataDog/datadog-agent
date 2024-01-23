// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bootstrap implements 'updater bootstrap'.
package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/cmd/updater/command"
	"github.com/DataDog/datadog-agent/pkg/updater"

	"github.com/spf13/cobra"
)

// Commands returns the bootstrap command
func Commands(global *command.GlobalParams) []*cobra.Command {
	var timeout time.Duration
	bootstrapCmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstraps the package with the first version.",
		Long: `Installs the first version of the package managed by this updater.
		This first version is sent remotely to the agent and can be configured from the UI.
		This command will exit after the first version is installed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return bootstrap(global.Package, global.RepositoriesDir, global.RunPath, timeout)
		},
	}
	bootstrapCmd.Flags().DurationVarP(&timeout, "timeout", "T", 3*time.Minute, "timeout to bootstrap with")
	return []*cobra.Command{bootstrapCmd}
}

func bootstrap(pkg string, defaultRepositoriesPath string, defaultRunPath string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	orgConfig, err := updater.NewOrgConfig()
	if err != nil {
		return fmt.Errorf("could not create org config: %w", err)
	}
	err = updater.Install(ctx, orgConfig, pkg, defaultRepositoriesPath, defaultRunPath)
	if err != nil {
		return fmt.Errorf("could not install package: %w", err)
	}
	return nil
}

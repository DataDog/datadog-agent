// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package subcommands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/apminject"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func uninstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall APM instrumentation",
		Long:  `Removes APM instrumentation from the host and/or Docker containers.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()

			log.Info("Starting APM injector removal...")

			installer := apminject.NewInstaller()
			defer installer.Finish(nil)

			if err := installer.Remove(ctx); err != nil {
				return fmt.Errorf("failed to remove APM injector: %w", err)
			}

			log.Info("APM injector removed successfully")
			return nil
		},
	}

	return cmd
}

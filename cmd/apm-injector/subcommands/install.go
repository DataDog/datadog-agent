// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package subcommands implements the subcommands for the apm-injector.
package subcommands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/apminject"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func installCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install APM instrumentation",
		Long:  `Installs APM instrumentation on the host and/or Docker containers.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()

			log.Info("Starting APM injector installation...")

			installer := apminject.NewInstaller()
			defer installer.Finish(nil)

			if err := installer.Setup(ctx); err != nil {
				installer.Finish(err)
				return fmt.Errorf("failed to setup APM injector: %w", err)
			}

			log.Info("APM injector installed successfully")
			return nil
		},
	}

	return cmd
}

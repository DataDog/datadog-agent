// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package containermode resolves which Private Action Runner topology a container should launch.
package containermode

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	monolithicMode = "monolithic"
	splitMode      = "split"
)

// Commands returns the internal container-mode resolver subcommand.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := &cobra.Command{
		Use:    "resolve-container-mode",
		Short:  "Resolve the Private Action Runner container deployment mode",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(run,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath)),
					LogParams:    log.ForOneShot(command.LoggerName, "off", false),
				}),
				core.Bundle(),
			)
		},
	}
	return []*cobra.Command{cmd}
}

func run(cfg config.Component) error {
	return writeMode(cfg, os.Stdout)
}

func writeMode(cfg config.Component, out io.Writer) error {
	mode := monolithicMode
	if cfg.GetBool(privateactionrunner.PAREnabled) && cfg.GetBool(privateactionrunner.PARSplitEnabled) {
		mode = splitMode
	}
	_, err := fmt.Fprintln(out, mode)
	return err
}

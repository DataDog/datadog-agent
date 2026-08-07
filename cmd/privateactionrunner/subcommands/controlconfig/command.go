// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package controlconfig implements the internal effective-config handoff used
// by the Rust Private Action Runner control plane.
package controlconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	parcontrolconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/controlconfig"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Commands returns the internal config-resolver subcommand used by par-control.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := &cobra.Command{
		Use:    "resolve-control-config",
		Short:  "Resolve configuration for the split Private Action Runner control plane",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(
				run,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath)),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true),
				}),
				core.Bundle(core.WithSecrets()),
				hostnameimpl.Module(),
			)
		},
	}
	return []*cobra.Command{cmd}
}

func run(cfg config.Component, hostnameComp hostname.Component) error {
	ctx := context.Background()
	agentIdentifier, err := enrollment.GetAgentIdentifier(ctx, hostnameComp)
	if err != nil {
		return fmt.Errorf("resolve Agent identity: %w", err)
	}
	effective, err := parcontrolconfig.ResolveWithIdentity(ctx, cfg, agentIdentifier)
	if err != nil {
		return fmt.Errorf("resolve Private Action Runner identity: %w", err)
	}
	return json.NewEncoder(os.Stdout).Encode(effective)
}

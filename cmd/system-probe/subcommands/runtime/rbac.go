// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package runtime holds runtime related files
package runtime

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

func rbacCommands(globalParams *command.GlobalParams) []*cobra.Command {
	rbacCmd := &cobra.Command{
		Use: "rbac",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(rbacRun,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams("", config.WithConfigMissingOK(true)),
					SecretParams: secrets.NewDisabledParams(),
					LogParams:    log.ForOneShot("SYS-PROBE", "off", false)}),
				core.Bundle(),
			)
		},
	}

	return []*cobra.Command{rbacCmd}
}

func rbacRun(_ log.Component, _ config.Component, _ secrets.Component) {
	fmt.Println("HELLO RBAC")
}

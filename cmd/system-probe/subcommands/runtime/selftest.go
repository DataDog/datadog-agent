// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package runtime

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

// SelfTestCommand returns the CLI command for "runtime self-test"
func SelfTestCommand(globalParams *command.GlobalParams) *cobra.Command {
	return &cobra.Command{
		Use:   "self-test",
		Short: "Run runtime self test",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(RunRuntimeSelfTest,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams("", config.WithConfigMissingOK(true)),
					SecretParams: secrets.NewDisabledParams(),
					LogParams:    log.ForOneShot(globalParams.LoggerName, "info", true)}),
				core.Bundle(),
			)
		},
	}
}

// RunRuntimeSelfTest runs the runtime self test
func RunRuntimeSelfTest(_ log.Component, _ config.Component, _ secrets.Component) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	selfTestResult, err := client.RunSelfTest()
	if err != nil {
		return fmt.Errorf("unable to get a process self test: %w", err)
	}

	if selfTestResult.Ok {
		fmt.Printf("Runtime self test: OK\n")
	} else {
		fmt.Printf("Runtime self test: error: %v\n", selfTestResult.Error)
	}
	return nil
}

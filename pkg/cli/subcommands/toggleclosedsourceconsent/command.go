// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

// Package toggleclosedsourceconsent implements 'agent AllowClosedSource=[true|false]
package toggleclosedsourceconsent

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	GlobalParams
	args []string
}

type GlobalParams struct {
	ConfFilePath   string
	ConfigName     string
	LoggerName     string
	SettingsClient func() (settings.Client, error)
}

// Commands returns a slice of subcommands for the 'agent' command.
func MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command {
	cliParams := &cliParams{}
	oneShotRunE := func(callback interface{}) func(cmd *cobra.Command, args []string) error {
		return func(cmd *cobra.Command, args []string) error {
			globalParams := globalParamsGetter()

			cliParams.args = args
			cliParams.GlobalParams = globalParams

			return fxutil.OneShot(callback,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParamsWithoutSecrets(globalParams.ConfFilePath, config.WithConfigName(globalParams.ConfigName)),
					LogParams:    log.LogForOneShot(globalParams.LoggerName, "off", true)}),
				core.Bundle,
			)
		}
	}

	cmd := &cobra.Command{
		Use:   "closedsourceconsent",
		Short: "Print the current closed source consent",
		Long:  ``,
		RunE:  oneShotRunE(showClosedSourceConsent),
	}

	setClosedSourceConsentCmd := &cobra.Command{
		Use:   "set [true | false]",
		Short: "Set closed source consent to true or false (agent service restart required for changes to take effect)",
		Long:  "",
		RunE:  oneShotRunE(setClosedSourceConsent),
	}
	// setClosedSourceConsentCmd.Flags().BoolVar()
	cmd.AddCommand(setClosedSourceConsentCmd)
	return cmd
}

func showClosedSourceConsent() {
	consentVal, _ := winutil.GetClosedSourceConsent()
	if consentVal == winutil.ClosedSourceAllowed {
		fmt.Printf("Consent Allowed: %d", consentVal)
	} else {
		fmt.Printf("Consent Denied: %d", consentVal)
	}
}

func setClosedSourceConsent(log log.Component, config config.Component, cliParams *cliParams) (err error) {
	consent, err := strconv.ParseBool(cliParams.args[0])
	if err != nil {
		fmt.Printf("Invalid parameter passed: %s", cliParams.args[0])
	}

	if consent {
		err = winutil.AllowClosedSource()
	} else {
		err = winutil.DenyClosedSource()
	}
	if err != nil {
		fmt.Printf("Unable to update closed source consent: %v", err)
	}
	showClosedSourceConsent()
	return
}

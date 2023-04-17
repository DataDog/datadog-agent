// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

// Package closedsourceconsent implements 'agent closedsourceconsent set [true|false]'.
package closedsourceconsent

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	GlobalParams
	args []string
}

type GlobalParams struct {
	ConfFilePath string
	ConfigName   string
	LoggerName   string
}

// MakeCommand returns a slice of subcommands for the 'agent' command.
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
		Use:   "set [true|false]",
		Short: "Set closed source consent to true or false (agent restart required for changes to take effect)",
		Long:  "Consent to closed source Datadog software to be installed and run on the host",
		RunE:  oneShotRunE(setClosedSourceConsent),
	}
	// setClosedSourceConsentCmd.Flags().BoolVar()
	cmd.AddCommand(setClosedSourceConsentCmd)
	return cmd
}

func showClosedSourceConsent() {
	consentVal, err := winutil.GetClosedSourceConsent()

	// couldn't get the value
	if err != nil {
		fmt.Printf("Unable to retrieve current closed source consent option: %v", err)
		return
	}

	// allowed
	if consentVal == winutil.ClosedSourceAllowed {
		fmt.Printf("Consent Allowed: %d", consentVal)
	} else if consentVal == winutil.ClosedSourceDenied { // denied
		fmt.Printf("Consent Denied: %d", consentVal)
	} else { // unknown
		fmt.Printf("Unknown consent value: %d", consentVal)
	}
}

func setClosedSourceConsent(log log.Component, config config.Component, cliParams *cliParams) (err error) {

	// only expect 1 arg
	if len(cliParams.args) != 1 {
		return fmt.Errorf("Too many arguments provided. Expected %d, received %d", 1, len(cliParams.args))
	}

	// make sure its a bool
	consent, err := strconv.ParseBool(cliParams.args[0])
	if err != nil {
		return fmt.Errorf("Invalid parameter passed: %s", cliParams.args[0])
	}

	// update
	err = winutil.SetClosedSourceAllowed(consent)
	if err != nil {
		err = fmt.Errorf("Unable to update closed source consent: %v", err)
	}

	// show the current value
	showClosedSourceConsent()
	return
}

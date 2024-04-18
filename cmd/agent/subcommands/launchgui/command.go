// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package launchgui implements 'agent launch-gui'.
package launchgui

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	launchCmd := &cobra.Command{
		Use:   "launch-gui",
		Short: "starts the Datadog Agent GUI",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(launchGui,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath)}),
				core.Bundle(),
			)
		},
		SilenceUsage: true,
	}

	return []*cobra.Command{launchCmd}
}

func launchGui(config config.Component, _ *cliParams) error {
	guiPort := pkgconfig.Datadog.GetString("GUI_port")
	if guiPort == "-1" {
		return fmt.Errorf("GUI not enabled: to enable, please set an appropriate port in your datadog.yaml file")
	}

	// Read the authentication token: can only be done if user can read from datadog.yaml
	authToken, err := security.FetchAuthToken(config)
	if err != nil {
		return err
	}

	// Create a new token object, specifying signing method and the claims
	// you would like it to contain.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	})

	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString([]byte(authToken))
	if err != nil {
		return fmt.Errorf("error generating JWT: " + err.Error())
	}

	// Open the GUI in a browser, passing the authorization tokens as parameters
	err = open("http://127.0.0.1:" + guiPort + "/?authToken=" + tokenString)
	if err != nil {
		return fmt.Errorf("error opening GUI: " + err.Error())
	}

	fmt.Printf("GUI opened at 127.0.0.1:" + guiPort + "\n")
	return nil
}

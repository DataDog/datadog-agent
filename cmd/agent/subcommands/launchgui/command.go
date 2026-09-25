// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package launchgui implements 'agent launch-gui'.
package launchgui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/gui/bootstrap"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/system"
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
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(launchGui,
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
		SilenceUsage: true,
	}

	return []*cobra.Command{launchCmd}
}

func launchGui(config config.Component, _ *cliParams, _ log.Component, client ipc.HTTPClient) error {
	guiPort := config.GetString("GUI_port")
	if guiPort == "-1" {
		return errors.New("GUI not enabled: to enable, please set an appropriate port in your datadog.yaml file")
	}

	// 'http://localhost' is preferred over 'http://127.0.0.1' due to Internet Explorer behavior.
	// Internet Explorer High Security Level does not support setting cookies via HTTP Header response.
	// By default, 'http://localhost' is categorized as an "intranet" website, which is considered safer and allowed to use cookies. This is not the case for 'http://127.0.0.1'.
	guiHost, err := system.IsLocalAddress(config.GetString("GUI_host"))
	if err != nil {
		return fmt.Errorf("GUI server host is not a local address: %s", err)
	}

	endpoint, err := client.NewIPCEndpoint("/agent/gui/intent")
	if err != nil {
		return err
	}

	res, err := endpoint.DoGet()
	if err != nil {
		return err
	}

	var intentToken bootstrap.Token
	if err := json.Unmarshal(res, &intentToken); err != nil {
		return fmt.Errorf("unable to decode the GUI intent token: %w", err)
	}

	guiAddress := net.JoinHostPort(guiHost, guiPort)

	// Hand the intent token to the browser through a file only this user can
	// read, rather than through the URL: a URL ends up in the argv of the OS
	// URL-opener and of the browser it spawns, where any other local user could
	// read it (VULN-92705).
	bootstrap.Sweep()
	pageURL, err := bootstrap.Write(guiAddress, intentToken)
	if err != nil {
		return err
	}

	if err := open(pageURL); err != nil {
		return fmt.Errorf("error opening GUI: %w (open %s to continue manually)", err, pageURL)
	}

	fmt.Printf("GUI opened at %s\n", guiAddress)
	return nil
}

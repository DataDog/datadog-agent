// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	_ "embed"
	"fmt"
	"html/template"
	"os"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/updater/localapiclient"
	"github.com/DataDog/datadog-agent/comp/updater/localapiclient/localapiclientimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

func statusCommand(global *command.GlobalParams) *cobra.Command {
	statusCmd := &cobra.Command{
		Use:     "status",
		Short:   "Print the installer status",
		GroupID: "daemon",
		Long:    ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return statusFxWrapper(global)
		},
	}
	return statusCmd
}

func statusFxWrapper(global *command.GlobalParams) error {
	return fxutil.OneShot(status,
		fx.Supply(core.BundleParams{
			ConfigParams:         config.NewAgentParams(global.ConfFilePath),
			SecretParams:         secrets.NewEnabledParams(),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(),
			LogParams:            logimpl.ForOneShot("INSTALLER", "off", true),
		}),
		core.Bundle(),
		localapiclientimpl.Module(),
	)
}

//go:embed status.tmpl
var statusTmpl []byte

var functions = template.FuncMap{
	"greenText":  color.GreenString,
	"yellowText": color.YellowString,
	"redText":    color.RedString,
	"boldText":   color.New(color.Bold).Sprint,
	"italicText": color.New(color.Italic).Sprint,
	"htmlSafe": func(html string) template.HTML {
		return template.HTML(html)
	},
}

func status(client localapiclient.Component) error {
	tmpl, err := template.New("status").Funcs(functions).Parse(string(statusTmpl))
	if err != nil {
		return fmt.Errorf("error parsing status template: %w", err)
	}
	status, err := client.Status()
	if err != nil {
		return fmt.Errorf("error getting status: %w", err)
	}
	err = tmpl.Execute(os.Stdout, status)
	if err != nil {
		return fmt.Errorf("error executing status template: %w", err)
	}
	return nil
}

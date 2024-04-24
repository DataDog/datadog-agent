// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	_ "embed"
	"fmt"
	"os"
	"text/template"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/comp/updater/localapiclient"
	"github.com/DataDog/datadog-agent/comp/updater/localapiclient/localapiclientimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func statusCommand(_ *command.GlobalParams) *cobra.Command {
	statusCmd := &cobra.Command{
		Use:     "status",
		Short:   "Print the installer status",
		GroupID: "daemon",
		Long:    ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return statusFxWrapper()
		},
	}
	return statusCmd
}

func statusFxWrapper() error {
	return fxutil.OneShot(status,
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

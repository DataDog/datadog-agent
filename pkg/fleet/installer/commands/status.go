// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	template "github.com/DataDog/datadog-agent/pkg/template/html"
)

func statusCommand() *cobra.Command {
	var debug bool
	var jsonOutput bool

	statusCmd := &cobra.Command{
		Use:     "status",
		Short:   "Print the installer status",
		GroupID: "installer",
		RunE: func(_ *cobra.Command, _ []string) error {
			return status(debug, jsonOutput)
		},
	}

	statusCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug mode")
	statusCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	return statusCmd
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

func status(debug bool, jsonOutput bool) error {
	tmpl, err := template.New("status").Funcs(functions).Parse(string(statusTmpl))
	if err != nil {
		return fmt.Errorf("error parsing status template: %w", err)
	}

	i, err := newInstallerCmd("get_states")
	if err != nil {
		return err
	}
	defer i.stop(err)
	status, err := i.Status(i.ctx, debug)
	if err != nil {
		return err
	}

	if !jsonOutput {
		err = tmpl.Execute(os.Stdout, status)
		if err != nil {
			return fmt.Errorf("error executing status template: %w", err)
		}
	} else {
		rawResult, err := json.Marshal(status)
		if err != nil {
			return fmt.Errorf("error marshalling status response: %w", err)
		}
		fmt.Println(string(rawResult))
	}
	return nil
}

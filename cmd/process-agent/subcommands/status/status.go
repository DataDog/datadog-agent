// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package status

import (
	"fmt"
	"io"
	"os"
	"text/template"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	compStatus "github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/process"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

var httpClient = apiutil.GetClient(false)

const (
	notRunning = `
=============
Process Agent
=============

  The Process Agent is not running

`

	errorMessage = `
=====
Error
=====

{{ . }}

`
)

type cliParams struct {
	*command.GlobalParams
}

type dependencies struct {
	fx.In

	CliParams *cliParams

	Config config.Component
	Log    log.Component
}

// Commands returns a slice of subcommands for the `status` command in the Process Agent
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	// statusCmd is a cobra command that prints the current status
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Print the current status",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(runStatus,
				fx.Supply(cliParams, command.GetCoreBundleParamsForOneShot(globalParams)),
				fx.Supply(
					compStatus.Params{
						PythonVersionGetFunc: python.GetPythonVersion,
					},
				),
				core.Bundle(),
				process.Bundle(),
			)
		},
	}

	return []*cobra.Command{statusCmd}
}

func writeNotRunning(log log.Component, w io.Writer) {
	_, err := fmt.Fprint(w, notRunning)
	if err != nil {
		_ = log.Error(err)
	}
}

func writeError(log log.Component, w io.Writer, e error) {
	tpl, err := template.New("").Funcs(compStatus.TextFmap()).Parse(errorMessage)
	if err != nil {
		_ = log.Error(err)
	}

	err = tpl.Execute(w, e)
	if err != nil {
		_ = log.Error(err)
	}
}

func fetchStatus(statusURL string) ([]byte, error) {
	body, err := apiutil.DoGet(httpClient, statusURL, apiutil.LeaveConnectionOpen)
	if err != nil {
		return nil, status.NewConnectionError(err)
	}

	return body, nil
}

// getAndWriteStatus calls the status server and writes it to `w`
func getAndWriteStatus(log log.Component, statusURL string, w io.Writer) {
	body, err := fetchStatus(statusURL)
	if err != nil {
		writeNotRunning(log, w)
		return
	}

	_, err = w.Write([]byte(body))
	if err != nil {
		_ = log.Error(err)
	}
}

func getStatusURL() (string, error) {
	addressPort, err := ddconfig.GetProcessAPIAddressPort()
	if err != nil {
		return "", fmt.Errorf("config error: %s", err.Error())
	}
	return fmt.Sprintf("http://%s/agent/status", addressPort), nil
}

func runStatus(deps dependencies) error {
	statusURL, err := getStatusURL()
	if err != nil {
		writeError(deps.Log, os.Stdout, err)
		return err
	}

	getAndWriteStatus(deps.Log, statusURL, os.Stdout)
	return nil
}

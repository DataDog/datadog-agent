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

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compStatus "github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/process"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/process/util/status"
	template "github.com/DataDog/datadog-agent/pkg/template/text"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

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
	Client ipc.HTTPClient
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
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(runStatus,
				fx.Supply(cliParams, command.GetCoreBundleParamsForOneShot(globalParams)),
				fx.Supply(
					compStatus.Params{
						PythonVersionGetFunc: python.GetPythonVersion,
					},
				),
				core.Bundle(),
				process.Bundle(),
				ipcfx.ModuleReadOnly(),
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

func fetchStatus(c ipc.HTTPClient, statusURL string) ([]byte, error) {
	body, err := c.Get(statusURL, ipchttp.WithLeaveConnectionOpen)
	if err != nil {
		return nil, status.NewConnectionError(err)
	}

	return body, nil
}

// getAndWriteStatus calls the status server and writes it to `w`
func getAndWriteStatus(log log.Component, c ipc.HTTPClient, statusURL string, w io.Writer) {
	body, err := fetchStatus(c, statusURL)
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
	addressPort, err := pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
	if err != nil {
		return "", fmt.Errorf("config error: %s", err.Error())
	}
	return fmt.Sprintf("https://%s/agent/status", addressPort), nil
}

func runStatus(deps dependencies) error {
	statusURL, err := getStatusURL()
	if err != nil {
		writeError(deps.Log, os.Stdout, err)
		return err
	}

	getAndWriteStatus(deps.Log, deps.Client, statusURL, os.Stdout)
	return nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/process-agent/api"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	ddstatus "github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

func writeNotRunning(w io.Writer) {
	_, err := fmt.Fprint(w, notRunning)
	if err != nil {
		_ = log.Error(err)
	}
}

func writeError(w io.Writer, e error) {
	tpl, err := template.New("").Funcs(ddstatus.Textfmap()).Parse(errorMessage)
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
		return nil, util.NewConnectionError(err)
	}

	return body, nil
}

// getAndWriteStatus calls the status server and writes it to `w`
func getAndWriteStatus(statusURL string, w io.Writer, options ...util.StatusOption) {
	body, err := fetchStatus(statusURL)
	if err != nil {
		switch err.(type) {
		case util.ConnectionError:
			writeNotRunning(w)
		default:
			writeError(w, err)
		}
		return
	}

	// If options to override the status are provided, we need to deserialize and serialize it again
	if len(options) > 0 {
		var s util.Status
		err = json.Unmarshal(body, &s)
		if err != nil {
			writeError(w, err)
			return
		}

		for _, option := range options {
			option(&s)
		}

		body, err = json.Marshal(s)
		if err != nil {
			writeError(w, err)
			return
		}
	}

	stats, err := ddstatus.FormatProcessAgentStatus(body)
	if err != nil {
		writeError(w, err)
		return
	}

	_, err = w.Write([]byte(stats))
	if err != nil {
		_ = log.Error(err)
	}
}

func getStatusURL() (string, error) {
	addressPort, err := api.GetAPIAddressPort()
	if err != nil {
		return "", fmt.Errorf("config error: %s", err.Error())
	}
	return fmt.Sprintf("http://%s/agent/status", addressPort), nil
}

// StatusCmd returns a cobra command that prints the current status
func StatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print the current status",
		Long:  ``,
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, _ []string) error {
	err := config.LoadConfigIfExists(cmd.Flag("config").Value.String())
	if err != nil {
		writeError(os.Stdout, err)
		return err
	}

	err = ddconfig.SetupLogger(
		"process",
		ddconfig.Datadog.GetString("log_level"),
		"",
		"",
		false,
		true,
		false,
	)
	if err != nil {
		writeError(os.Stdout, err)
		return err
	}

	statusURL, err := getStatusURL()
	if err != nil {
		writeError(os.Stdout, err)
		return err
	}

	getAndWriteStatus(statusURL, os.Stdout)
	return nil
}

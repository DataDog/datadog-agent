// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package streamlogs implements 'agent stream-logs'.
package streamlogs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	filters diagnostic.Filters
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	cmd := &cobra.Command{
		Use:   "stream-logs",
		Short: "Stream the logs being processed by a running agent",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(streamLogs,
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle,
			)
		},
	}
	cmd.Flags().StringVar(&cliParams.filters.Name, "name", "", "Filter by name")
	cmd.Flags().StringVar(&cliParams.filters.Type, "type", "", "Filter by type")
	cmd.Flags().StringVar(&cliParams.filters.Source, "source", "", "Filter by source")
	cmd.Flags().StringVar(&cliParams.filters.Service, "service", "", "Filter by service")

	return []*cobra.Command{cmd}
}

func streamLogs(log log.Component, config config.Component, cliParams *cliParams) error {
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return err
	}

	body, err := json.Marshal(&cliParams.filters)

	if err != nil {
		return err
	}

	urlstr := fmt.Sprintf("https://%v:%v/agent/stream-logs", ipcAddress, config.GetInt("cmd_port"))
	return streamRequest(urlstr, body, func(chunk []byte) {
		fmt.Print(string(chunk))
	})
}

func streamRequest(url string, body []byte, onChunk func([]byte)) error {
	var e error
	c := util.GetClient(false)

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		return e
	}

	e = util.DoPostChunked(c, url, "application/json", bytes.NewBuffer(body), onChunk)

	if e == io.EOF {
		return nil
	}
	if e != nil {
		fmt.Printf("Could not reach agent: %v \nMake sure the agent is running before requesting the logs and contact support if you continue having issues. \n", e)
	}
	return e
}

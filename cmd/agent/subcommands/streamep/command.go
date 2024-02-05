// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package streamep implements 'agent stream-event-platform'.
package streamep

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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
		Use:   "stream-event-platform",
		Short: "Stream the event platform payloads being processed by a running agent",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(streamEventPlatform,
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
			)
		},
	}
	cmd.Flags().StringVar(&cliParams.filters.Type, "type", "", "Filter by type")

	return []*cobra.Command{cmd}
}

//nolint:revive // TODO(CINT) Fix revive linter
func streamEventPlatform(log log.Component, config config.Component, cliParams *cliParams) error {
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return err
	}

	body, err := json.Marshal(&cliParams.filters)

	if err != nil {
		return err
	}

	urlstr := fmt.Sprintf("https://%v:%v/agent/stream-event-platform", ipcAddress, config.GetInt("cmd_port"))
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
		fmt.Printf("Could not reach agent: %v \nMake sure the agent is running before requesting the event platform stream and contact support if you continue having issues. \n", e)
	}
	return e
}

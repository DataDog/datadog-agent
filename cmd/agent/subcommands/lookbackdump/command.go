// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lookbackdump implements 'agent metric-lookback-dump'.
package lookbackdump

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams
}

// lookbackDumpResponse mirrors the JSON returned by the /metric-lookback-dump endpoint.
type lookbackDumpResponse struct {
	SeriesDumped int `json:"series_dumped"`
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	cmd := &cobra.Command{
		Use:   "metric-lookback-dump",
		Short: "Flush the retained metric lookback buffer through the serializer",
		Long: `Sends every sample currently retained in the in-memory metric lookback ` +
			`ring buffer to the Datadog backend via the running agent's normal ` +
			`serializer path. Requires metric_lookback.enabled to be set.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(requestLookbackDump,
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	return []*cobra.Command{cmd}
}

func requestLookbackDump(_ log.Component, config config.Component, _ *cliParams, client ipc.HTTPClient) error {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return err
	}
	urlstr := fmt.Sprintf("https://%s/agent/metric-lookback-dump",
		net.JoinHostPort(ipcAddress, strconv.Itoa(config.GetInt("cmd_port"))))

	body, err := client.Post(urlstr, "application/json", nil)
	if err != nil {
		// Surface the agent-provided error message when present.
		errMap := make(map[string]string)
		if json.Unmarshal(body, &errMap) == nil {
			if msg, found := errMap["error"]; found {
				return errors.New(msg)
			}
		}
		fmt.Printf("Could not reach the agent: %v\nMake sure the agent is running before requesting a lookback dump.\n", err)
		return err
	}

	var resp lookbackDumpResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return err
	}

	fmt.Printf("Dumped %d metric lookback series to the serializer.\n", resp.SeriesDumped)
	return nil
}

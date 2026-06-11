// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lookbackdump implements the metric lookback debugging commands.
package lookbackdump

import (
	"bytes"
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

// cliParams are the command-line arguments for metric-lookback-dump.
type cliParams struct {
	*command.GlobalParams
}

// seedParams are the command-line arguments for metric-lookback-seed.
type seedParams struct {
	*command.GlobalParams
	checkID    string
	metric     string
	value      float64
	hostname   string
	tags       []string
	metricType string
}

// lookbackDumpResponse mirrors the JSON returned by the /metric-lookback-dump endpoint.
type lookbackDumpResponse struct {
	SeriesDumped int `json:"series_dumped"`
}

// lookbackSeedRequest mirrors the JSON accepted by the /metric-lookback-seed endpoint.
type lookbackSeedRequest struct {
	CheckID  string   `json:"check_id"`
	Metric   string   `json:"metric"`
	Value    float64  `json:"value"`
	Hostname string   `json:"hostname,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Type     string   `json:"type,omitempty"`
}

// lookbackSeedResponse mirrors the JSON returned by the /metric-lookback-seed endpoint.
type lookbackSeedResponse struct {
	CheckID         string `json:"check_id"`
	Metric          string `json:"metric"`
	Type            string `json:"type"`
	SamplesBuffered int    `json:"samples_buffered"`
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{GlobalParams: globalParams}

	dumpCmd := &cobra.Command{
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

	seedParams := &seedParams{
		GlobalParams: globalParams,
		checkID:      "demo-shadow",
		metric:       "demo.lookback.shadow",
		value:        42,
		tags:         []string{"demo:lookback"},
		metricType:   "gauge",
	}
	seedCmd := &cobra.Command{
		Use:    "metric-lookback-seed",
		Short:  "Seed the metric lookback buffer for a live demo",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(requestLookbackSeed,
				fx.Supply(seedParams),
				fx.Supply(command.GetDefaultCoreBundleParams(seedParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}
	seedCmd.Flags().StringVar(&seedParams.checkID, "check-id", seedParams.checkID, "shadow check ID to seed")
	seedCmd.Flags().StringVar(&seedParams.metric, "metric", seedParams.metric, "metric name to seed")
	seedCmd.Flags().Float64Var(&seedParams.value, "value", seedParams.value, "metric value to seed")
	seedCmd.Flags().StringVar(&seedParams.hostname, "hostname", seedParams.hostname, "metric hostname to seed (empty uses the agent default)")
	seedCmd.Flags().StringSliceVar(&seedParams.tags, "tag", seedParams.tags, "metric tag(s) to seed; repeat or comma-separate")
	seedCmd.Flags().StringVar(&seedParams.metricType, "type", seedParams.metricType, "metric type to seed: gauge, count, or rate")

	return []*cobra.Command{dumpCmd, seedCmd}
}

func requestLookbackDump(_ log.Component, config config.Component, _ *cliParams, client ipc.HTTPClient) error {
	urlstr, err := agentEndpointURL(config, "metric-lookback-dump")
	if err != nil {
		return err
	}

	body, err := client.Post(urlstr, "application/json", nil)
	if err != nil {
		return postError(body, err, "requesting a lookback dump")
	}

	var resp lookbackDumpResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return err
	}

	fmt.Printf("Dumped %d metric lookback series to the serializer.\n", resp.SeriesDumped)
	return nil
}

func requestLookbackSeed(_ log.Component, config config.Component, params *seedParams, client ipc.HTTPClient) error {
	urlstr, err := agentEndpointURL(config, "metric-lookback-seed")
	if err != nil {
		return err
	}

	req := lookbackSeedRequest{
		CheckID:  params.checkID,
		Metric:   params.metric,
		Value:    params.value,
		Hostname: params.hostname,
		Tags:     params.tags,
		Type:     params.metricType,
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}

	body, err := client.Post(urlstr, "application/json", bytes.NewReader(payload))
	if err != nil {
		return postError(body, err, "seeding the metric lookback buffer")
	}

	var resp lookbackSeedResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return err
	}

	fmt.Printf("Seeded %d %s sample(s) for %q into the metric lookback buffer (check_id=%q).\n",
		resp.SamplesBuffered, resp.Type, resp.Metric, resp.CheckID)
	return nil
}

func agentEndpointURL(config config.Component, endpoint string) (string, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("https://%s/agent/%s", net.JoinHostPort(ipcAddress, strconv.Itoa(config.GetInt("cmd_port"))), endpoint), nil
}

func postError(body []byte, err error, action string) error {
	// Surface the agent-provided error message when present.
	errMap := make(map[string]string)
	if json.Unmarshal(body, &errMap) == nil {
		if msg, found := errMap["error"]; found {
			return errors.New(msg)
		}
	}
	fmt.Printf("Could not reach the agent while %s: %v\nMake sure the agent is running.\n", action, err)
	return err
}

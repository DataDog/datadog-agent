// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package run

import (
	"context"
	"log"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/stretchr/testify/require"
)

func TestFxRun(t *testing.T) {
	fxutil.TestRun(t, func() error {
		ctx := context.Background()
		cliParams := &subcommands.GlobalParams{}
		return runOTelAgentCommand(ctx, cliParams)
	})
}

func waitForReadiness() {
	for i := 0; ; i++ {
		resp, err := http.Get("http://localhost:13133") // default addr of the OTel collector health check extension
		defer func() {
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
		}()
		if err == nil && resp.StatusCode == 200 {
			return
		}
		log.Print("health check failed, retrying ", i, err, resp)
		t := time.Duration(math.Pow(2, float64(i)))
		time.Sleep(t * time.Second)
	}
}

func TestRunOTelAgentCommand(t *testing.T) {
	apmstatsRec := &testutil.HTTPRequestRecorderWithChan{Pattern: testutil.APMStatsEndpoint, ReqChan: make(chan []byte)}
	tracesRec := &testutil.HTTPRequestRecorderWithChan{Pattern: testutil.TraceEndpoint, ReqChan: make(chan []byte)}
	server := testutil.DatadogServerMock(apmstatsRec.HandlerFunc, tracesRec.HandlerFunc)
	defer server.Close()
	t.Setenv("SERVER_URL", server.URL)

	params := &subcommands.GlobalParams{
		ConfPaths:  []string{"test_config.yaml"},
		ConfigName: "datadog-otel",
		LoggerName: "OTELCOL",
	}
	go func() {
		err := runOTelAgentCommand(context.Background(), params)
		require.NoError(t, err)
	}()
	waitForReadiness()
}

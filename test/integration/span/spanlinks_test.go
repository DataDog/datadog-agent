// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package agent

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/stretchr/testify/assert"
)

func TestSpanLinks(t *testing.T) {
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	cfg.AgentVersion = "testVersion"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agnt := agent.NewAgent(
		ctx,
		cfg,
		telemetry.NewNoopCollector())
	agnt.Receiver.Start()

	testTraces := testutil.RandomTrace(10, 20)
	msgpBts, err := testTraces.MarshalMsg(nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, endpoint := range []string{
		"/v0.4/traces",
		"/v0.7/traces",
	} {
		req, err := http.NewRequest("POST", "http://localhost:8126"+endpoint, bytes.NewReader(msgpBts))
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.DefaultClient.Do(req)
			assert.NoError(t, err)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Error(resp.StatusCode)
				return
			}
		}()
	}
}

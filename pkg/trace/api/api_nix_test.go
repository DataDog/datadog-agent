// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package api

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/stretchr/testify/assert"
)

func TestUDS(t *testing.T) {
	sockPath := "/tmp/test-trace.sock"
	payload := msgpTraces(t, pb.Traces{testutil.RandomTrace(10, 20)})
	client := http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", sockPath)
			},
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	t.Run("off", func(t *testing.T) {
		conf := config.New()
		conf.Endpoints[0].APIKey = "apikey_2"

		r := newTestReceiverFromConfig(conf)
		r.Start()
		defer r.Stop()

		resp, err := client.Post("http://localhost:8126/v0.4/traces", "application/msgpack", bytes.NewReader(payload))
		if err == nil {
			resp.Body.Close()
			t.Fatalf("expected to fail, got response %#v", resp)
		}
	})

	t.Run("on", func(t *testing.T) {
		conf := config.New()
		conf.Endpoints[0].APIKey = "apikey_2"
		conf.ReceiverSocket = sockPath

		r := newTestReceiverFromConfig(conf)
		r.Start()
		defer r.Stop()

		resp, err := client.Post("http://localhost:8126/v0.4/traces", "application/msgpack", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected http.StatusOK, got response: %#v", resp)
		}
	})
}

func TestHTTPReceiverStart(t *testing.T) {
	var logs bytes.Buffer
	old := log.SetLogger(log.NewBufferLogger(&logs))
	defer log.SetLogger(old)

	for name, tt := range map[string]struct {
		port   int      // receiver port
		socket string   // socket
		out    []string // expected log output (uses strings.Contains)
	}{
		"off": {
			out: []string{"HTTP Server is off: all listeners are disabled"},
		},
		"tcp": {
			port: 8129,
			out: []string{
				"Listening for traces at http://localhost:8129",
			},
		},
		"uds": {
			socket: "/tmp/agent.sock",
			out: []string{
				"HTTP receiver disabled by config (apm_config.receiver_port: 0)",
				"Listening for traces at unix:///tmp/agent.sock",
			},
		},
		"both": {
			port:   8129,
			socket: "/tmp/agent.sock",
			out: []string{
				"Listening for traces at http://localhost:8129",
				"Listening for traces at unix:///tmp/agent.sock",
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			logs.Reset()
			cfg := config.New()
			cfg.ReceiverPort = tt.port
			cfg.ReceiverSocket = tt.socket
			r := newTestReceiverFromConfig(cfg)
			r.Start()
			defer r.Stop()
			for _, l := range tt.out {
				assert.Contains(t, logs.String(), l)
			}
		})
	}
}

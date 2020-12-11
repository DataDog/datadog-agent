// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package api

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
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
		if resp.StatusCode != 200 {
			t.Fatalf("expected http.StatusOK, got response: %#v", resp)
		}
	})
}

// FIXME: This test is disabled for WWindows because it fails in AppVeyor but we
// cannot reproduce it with a Windows VM setup.
func TestHandleStatsOff(t *testing.T) {
	cfg := newTestReceiverConfig()
	rcv := newTestReceiverFromConfig(cfg)
	mockProcessor := new(mockStatsProcessor)
	rcv.statsProcessor = mockProcessor
	rcv.Start()
	defer rcv.Stop()

	req, _ := http.NewRequest("POST", "http://127.0.0.1:8126/v0.5/stats", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 404 {
		t.Fail()
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package api

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
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
		// running the tests on different ports to prevent
		// flaky panics related to the port being already taken
		port := 8126
		conf := config.New()
		conf.Endpoints[0].APIKey = "apikey_2"
		conf.ReceiverPort = port

		r := newTestReceiverFromConfig(conf)
		r.Start()
		defer r.Stop()

		resp, err := client.Post(fmt.Sprintf("http://localhost:%v/v0.4/traces", port), "application/msgpack", bytes.NewReader(payload))
		if err == nil {
			resp.Body.Close()
			t.Fatalf("expected to fail, got response %#v", resp)
		}
	})

	t.Run("on", func(t *testing.T) {
		// running the tests on different ports to prevent
		// flaky panics related to the port being already taken
		port := 8125
		conf := config.New()
		conf.Endpoints[0].APIKey = "apikey_2"
		conf.ReceiverSocket = sockPath
		conf.ReceiverPort = port

		r := newTestReceiverFromConfig(conf)
		r.Start()
		defer r.Stop()

		resp, err := client.Post(fmt.Sprintf("http://localhost:%v/v0.4/traces", port), "application/msgpack", bytes.NewReader(payload))
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

	for name, setup := range map[string]func() (int, string, []string){
		"off": func() (int, string, []string) {
			return 0, "", []string{"HTTP Server is off: all listeners are disabled"}
		},
		"tcp": func() (int, string, []string) {
			port := freeTCPPort()
			return port, "", []string{fmt.Sprintf("Listening for traces at http://localhost:%d", port)}
		},
		"uds": func() (int, string, []string) {
			socket := filepath.Join(t.TempDir(), "agent.sock")
			return 0, socket, []string{
				"HTTP receiver disabled by config (apm_config.receiver_port: 0)",
				fmt.Sprintf("Listening for traces at unix://%s", socket),
			}
		},
		"both": func() (int, string, []string) {
			port := freeTCPPort()
			socket := filepath.Join(t.TempDir(), "agent.sock")
			return port, socket, []string{
				fmt.Sprintf("Listening for traces at http://localhost:%d", port),
				fmt.Sprintf("Listening for traces at unix://%s", socket),
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			logs.Reset()
			cfg := config.New()
			port, socket, out := setup()
			cfg.ReceiverPort = port
			cfg.ReceiverSocket = socket
			r := newTestReceiverFromConfig(cfg)
			r.Start()
			defer r.Stop()
			for _, l := range out {
				assert.Contains(t, logs.String(), l)
			}
		})
	}
}

// freePort returns a random and free TCP port.
func freeTCPPort() int {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("couldn't resolve address: %s", err))
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		panic(fmt.Sprintf("couldn't listen: %s", err))
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

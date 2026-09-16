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
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

func TestUDS(t *testing.T) {
	if os.Getenv("CI") == "true" && runtime.GOOS == "darwin" {
		t.Skip("TestUDS is known to fail on the macOS Gitlab runners because of the already running Agent")
	}
	sockPath := filepath.Join(t.TempDir(), "apm.sock")
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
		// The client's DialContext ignores the network/address it's given and
		// always dials sockPath, so the TCP host:port below is decorative —
		// only the receiver's own bind matters. Use a real listener rather
		// than a hardcoded port so this can't collide with another process.
		ln := testutil.TCPListener(t)
		tcpAddr := ln.Addr().(*net.TCPAddr)
		conf := config.New()
		conf.Endpoints[0].APIKey = "apikey_2"
		conf.ReceiverHost = tcpAddr.IP.String()
		conf.ReceiverPort = tcpAddr.Port
		conf.ReceiverSocket = ""

		r := newTestReceiverFromConfig(conf)
		r.SetTCPListener(ln)
		r.Start()
		defer r.Stop()

		resp, err := client.Post(fmt.Sprintf("http://localhost:%v/v0.4/traces", tcpAddr.Port), "application/msgpack", bytes.NewReader(payload))
		if err == nil {
			resp.Body.Close()
			t.Fatalf("expected to fail, got response %#v", resp)
		}
	})

	t.Run("on", func(t *testing.T) {
		// See the "off" subtest above: the TCP host:port is decorative here too.
		ln := testutil.TCPListener(t)
		tcpAddr := ln.Addr().(*net.TCPAddr)
		conf := config.New()
		conf.Endpoints[0].APIKey = "apikey_2"
		conf.ReceiverSocket = sockPath
		conf.ReceiverHost = tcpAddr.IP.String()
		conf.ReceiverPort = tcpAddr.Port

		r := newTestReceiverFromConfig(conf)
		r.SetTCPListener(ln)
		r.Start()
		defer r.Stop()

		resp, err := client.Post(fmt.Sprintf("http://localhost:%v/v0.4/traces", tcpAddr.Port), "application/msgpack", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected http.StatusOK, got response: %#v", resp)
		}
	})

	t.Run("uds_permission_err", func(t *testing.T) {
		dir := t.TempDir()
		err := os.Chmod(dir, 0444) // read-only
		assert.NoError(t, err)

		conf := config.New()
		conf.Endpoints[0].APIKey = "apikey_2"
		conf.ReceiverPort = 0 // do not bind production port 8126
		conf.ReceiverSocket = filepath.Join(dir, "apm.socket")

		r := newTestReceiverFromConfig(conf)
		// should not crash
		r.Start()
		r.Stop()
	})
}

func TestHTTPReceiverStart(t *testing.T) {
	var logs bytes.Buffer
	old := log.SetLogger(log.NewBufferLogger(&logs))
	defer log.SetLogger(old)

	for name, setup := range map[string]func(t *testing.T) (enabled bool, ln net.Listener, socket string, wantLogs func(addr string) []string){
		"disabled": func(_ *testing.T) (bool, net.Listener, string, func(string) []string) {
			return false, nil, "", func(string) []string {
				return []string{"HTTP Server is off: HTTPReceiver is disabled."}
			}
		},
		"off": func(_ *testing.T) (bool, net.Listener, string, func(string) []string) {
			return true, nil, "", func(string) []string {
				return []string{"HTTP Server is off: all listeners are disabled"}
			}
		},
		"tcp": func(t *testing.T) (bool, net.Listener, string, func(string) []string) {
			return true, testutil.TCPListener(t), "", func(addr string) []string {
				return []string{"Listening for traces at http://" + addr}
			}
		},
		"uds": func(t *testing.T) (bool, net.Listener, string, func(string) []string) {
			socket := filepath.Join(t.TempDir(), "agent.sock")
			return true, nil, socket, func(string) []string {
				return []string{
					"HTTP receiver disabled by config (apm_config.receiver_port: 0)",
					"Listening for traces at unix://" + socket,
				}
			}
		},
		"both": func(t *testing.T) (bool, net.Listener, string, func(string) []string) {
			socket := filepath.Join(t.TempDir(), "agent.sock")
			return true, testutil.TCPListener(t), socket, func(addr string) []string {
				return []string{
					"Listening for traces at http://" + addr,
					"Listening for traces at unix://" + socket,
				}
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			logs.Reset()
			cfg := config.New()
			enabled, ln, socket, wantLogs := setup(t)
			cfg.ReceiverEnabled = enabled
			cfg.ReceiverSocket = socket
			cfg.ReceiverPort = 0
			var addr string
			if ln != nil {
				tcpAddr := ln.Addr().(*net.TCPAddr)
				cfg.ReceiverHost = tcpAddr.IP.String()
				cfg.ReceiverPort = tcpAddr.Port
				addr = ln.Addr().String()
			}
			r := newTestReceiverFromConfig(cfg)
			if ln != nil {
				r.SetTCPListener(ln)
			}
			r.Start()
			defer r.Stop()
			for _, l := range wantLogs(addr) {
				assert.Contains(t, logs.String(), l)
			}
		})
	}
}

func TestShutdown(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "agent.sock")
	cfg := config.New()
	cfg.ReceiverEnabled = true
	cfg.ReceiverPort = 0
	cfg.ReceiverSocket = socket
	r := newTestReceiverFromConfig(cfg)
	r.Start()
	_, err := os.Stat(socket)
	assert.NoError(t, err)
	r.Stop()
	// Ensure we do not delete the socket
	_, err = os.Stat(socket)
	assert.NoError(t, err)
}

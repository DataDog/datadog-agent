// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/require"
)

func TestDogStatsDReverseProxy(t *testing.T) {
	testCases := []struct {
		name       string
		configFunc func(cfg *config.AgentConfig)
		errCode    int
	}{
		{
			"dogstatsd disabled",
			func(cfg *config.AgentConfig) {
				cfg.StatsdEnabled = false
			},
			http.StatusServiceUnavailable,
		},
		{
			"bad statsd host",
			func(cfg *config.AgentConfig) {
				cfg.StatsdHost = "this is invalid"
			},
			http.StatusInternalServerError,
		},
		{
			"bad statsd port",
			func(cfg *config.AgentConfig) {
				cfg.StatsdPort = -1
			},
			http.StatusInternalServerError,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.New()
			tc.configFunc(cfg)
			receiver := newTestReceiverFromConfig(cfg)
			proxy := receiver.dogstatsdProxyHandler()
			require.NotNil(t, proxy)

			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))
			require.Equal(t, tc.errCode, rec.Code)
		})
	}

	t.Run("dogstatsd enabled (default)", func(t *testing.T) {
		cfg := config.New()
		receiver := newTestReceiverFromConfig(cfg)
		proxy := receiver.dogstatsdProxyHandler()
		require.NotNil(t, proxy)

		rec := httptest.NewRecorder()
		body := ioutil.NopCloser(bytes.NewBufferString("users.online:1|c|@0.5|#country:china"))
		proxy.ServeHTTP(rec, httptest.NewRequest("POST", "/", body))
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestDogStatsDReverseProxyEndToEndUDP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	port, err := getAvailableUDPPort()
	if err != nil {
		t.Skip("Couldn't find available UDP port to run test. Skipping.")
	}
	cfg := config.New()
	cfg.StatsdHost = "127.0.0.1"
	cfg.StatsdPort = port

	address := fmt.Sprintf("%s:%d", cfg.StatsdHost, cfg.StatsdPort)
	ln, err := net.ListenPacket("udp", address)
	if err != nil {
		t.Fatalf("failed to create listener on %q: %v", address, err)
	}
	defer ln.Close()

	receiver := newTestReceiverFromConfig(cfg)
	proxy := receiver.dogstatsdProxyHandler()
	require.NotNil(t, proxy)
	rec := httptest.NewRecorder()

	// Send two payloads separated by a newline.
	payloads := [][]byte{[]byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"), []byte("_e{21,36}:An exception occurred|Cannot parse CSV file from\\n10.0.0.17|t:warning|#err_type:bad_file")}
	sep := []byte("\n")
	msg := bytes.Join(payloads, sep)
	body := ioutil.NopCloser(bytes.NewBuffer(msg))
	proxy.ServeHTTP(rec, httptest.NewRequest("POST", "/", body))
	require.Equal(t, http.StatusOK, rec.Code)

	// Check that both payloads were sent over (without a newline).
	ln.SetReadDeadline(time.Now().Add(30 * time.Second))
	buf := make([]byte, len(msg)-len(sep))
	n, _, err := ln.ReadFrom(buf)
	require.NoError(t, err)
	if got, want := buf[:n], payloads[0]; !bytes.Equal(got, want) {
		t.Errorf("got first payload: %q\nwant first payload: %q", got, want)
	}
	_, _, err = ln.ReadFrom(buf[n:])
	require.NoError(t, err)
	if got, want := buf[n:], payloads[1]; !bytes.Equal(got, want) {
		t.Errorf("got second payload: %q\nwant second payload: %q", got, want)
	}
}

// getAvailableUDPPort requests a random port number and makes sure it is available
func getAvailableUDPPort() (int, error) {
	// This is based on pkg/dogstatsd/server_test.go.
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return -1, fmt.Errorf("can't find an available udp port: %s", err)
	}
	defer conn.Close()

	_, portString, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return -1, fmt.Errorf("can't find an available udp port: %s", err)
	}
	portInt, err := strconv.Atoi(portString)
	if err != nil {
		return -1, fmt.Errorf("can't convert udp port: %s", err)
	}

	return portInt, nil
}

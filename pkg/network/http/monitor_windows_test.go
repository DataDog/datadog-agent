// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package http

import (
	"fmt"
	"math/rand"
	nethttp "net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/http/testutil"
	"github.com/stretchr/testify/require"
)

func TestHTTPMonitorIntegration(t *testing.T) {
	targetAddr := "localhost:8080"
	serverAddr := "localhost:8080"
	testHTTPMonitor(t, targetAddr, serverAddr, 100)
}

func testHTTPMonitor(t *testing.T, targetAddr, serverAddr string, numReqs int) {
	srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableTLS:        false,
		EnableKeepAlives: false,
	})
	defer srvDoneFn()

	di, err := newDriverInterface()
	require.NoError(t, err)

	telemetry := newTelemetry()
	monitor := &DriverMonitor{
		di:         di,
		telemetry:  telemetry,
		statkeeper: newHTTPStatkeeper(config.New(), telemetry),
	}

	monitor.Start()
	defer func() {
		err = monitor.Stop()
		require.NoError(t, err)
	}()

	// Perform a number of random requests
	requestFn := requestGenerator(t, targetAddr)
	var requests []*nethttp.Request
	for i := 0; i < numReqs; i++ {
		requests = append(requests, requestFn())
	}

	// Give the driver a moment to process the transactions
	time.Sleep(3 * time.Second)

	stats := monitor.GetHTTPStats()

	// Assert all requests made were correctly captured by the monitor
	for _, req := range requests {
		includesRequest(t, stats, req)
	}
}

func requestGenerator(t *testing.T, targetAddr string) func() *nethttp.Request {
	var (
		methods     = []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
		statusCodes = []int{200, 300, 400, 500}
		random      = rand.New(rand.NewSource(time.Now().Unix()))
		idx         = 0
		client      = new(nethttp.Client)
	)

	return func() *nethttp.Request {
		idx++
		method := methods[random.Intn(len(methods))]
		status := statusCodes[random.Intn(len(statusCodes))]
		url := fmt.Sprintf("http://%s/%d/request-%d", targetAddr, status, idx)
		req, err := nethttp.NewRequest(method, url, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		return req
	}
}

func includesRequest(t *testing.T, allStats map[Key]RequestStats, req *nethttp.Request) {
	expectedStatus := testutil.StatusFromPath(req.URL.Path)
	for key, stats := range allStats {
		i := expectedStatus/100 - 1
		if key.Path == req.URL.Path && stats[i].Count == 1 {
			return
		}
	}

	t.Errorf(
		"could not find HTTP transaction matching the following criteria:\n path=%s method=%s status=%d",
		req.URL.Path,
		req.Method,
		expectedStatus,
	)
}

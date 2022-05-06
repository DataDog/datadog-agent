// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"fmt"
	"math/rand"
	nethttp "net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/http/testutil"
	netlink "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/stretchr/testify/require"
)

func TestHTTPMonitorIntegration(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < kernel.VersionCode(4, 1, 0) {
		t.Skip("HTTP feature not available on pre 4.1.0 kernels")
	}

	targetAddr := "localhost:8080"
	serverAddr := "localhost:8080"
	testHTTPMonitor(t, targetAddr, serverAddr, 100)
}

func TestHTTPMonitorIntegrationWithNAT(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < kernel.VersionCode(4, 1, 0) {
		t.Skip("HTTP feature not available on pre 4.1.0 kernels")
	}

	// SetupDNAT sets up a NAT translation from 2.2.2.2 to 1.1.1.1
	netlink.SetupDNAT(t)
	defer netlink.TeardownDNAT(t)

	targetAddr := "2.2.2.2:8080"
	serverAddr := "1.1.1.1:8080"
	testHTTPMonitor(t, targetAddr, serverAddr, 10)
}

func TestUnknownMethodRegression(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < kernel.VersionCode(4, 1, 0) {
		t.Skip("HTTP feature not available on pre 4.1.0 kernels")
	}

	// SetupDNAT sets up a NAT translation from 2.2.2.2 to 1.1.1.1
	netlink.SetupDNAT(t)
	defer netlink.TeardownDNAT(t)

	targetAddr := "2.2.2.2:8080"
	serverAddr := "1.1.1.1:8080"
	srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableTLS:        false,
		EnableKeepAlives: true,
	})
	defer srvDoneFn()

	monitor, err := NewMonitor(config.New(), nil, nil)
	require.NoError(t, err)
	err = monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()

	requestFn := requestGenerator(t, targetAddr)
	for i := 0; i < 100; i++ {
		requestFn()
	}

	time.Sleep(10 * time.Millisecond)
	stats := monitor.GetHTTPStats()

	for key := range stats {
		if key.Method == MethodUnknown {
			t.Error("detected HTTP request with method unknown")
		}
	}

	telemetry := monitor.GetStats()
	require.NotEmpty(t, telemetry)
	_, ok := telemetry["dropped"]
	require.True(t, ok)
	_, ok = telemetry["misses"]
	require.True(t, ok)

}

func testHTTPMonitor(t *testing.T, targetAddr, serverAddr string, numReqs int) {
	srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableTLS:        false,
		EnableKeepAlives: false,
	})
	defer srvDoneFn()

	monitor, err := NewMonitor(config.New(), nil, nil)
	require.NoError(t, err)
	err = monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()

	// Perform a number of random requests
	requestFn := requestGenerator(t, targetAddr)
	var requests []*nethttp.Request
	for i := 0; i < numReqs; i++ {
		requests = append(requests, requestFn())
	}

	// Ensure all captured transactions get sent to user-space
	time.Sleep(10 * time.Millisecond)
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

func includesRequest(t *testing.T, allStats map[Key]*RequestStats, req *nethttp.Request) {
	expectedStatus := testutil.StatusFromPath(req.URL.Path)
	for key, stats := range allStats {
		if key.Path == req.URL.Path && stats.HasStats(expectedStatus) {
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

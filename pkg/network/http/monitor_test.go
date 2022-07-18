// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"fmt"
	"io"
	"math/rand"
	"net"
	nethttp "net/http"
	"net/url"
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

	t.Run("with keep-alives", func(t *testing.T) {
		testHTTPMonitor(t, targetAddr, serverAddr, 100, testutil.Options{
			EnableKeepAlives: true,
		})
	})
	t.Run("without keep-alives", func(t *testing.T) {
		testHTTPMonitor(t, targetAddr, serverAddr, 100, testutil.Options{
			EnableKeepAlives: false,
		})
	})
}

func TestHTTPMonitorIntegrationWithNAT(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < kernel.VersionCode(4, 1, 0) {
		t.Skip("HTTP feature not available on pre 4.1.0 kernels")
	}

	// SetupDNAT sets up a NAT translation from 2.2.2.2 to 1.1.1.1
	netlink.SetupDNAT(t)

	targetAddr := "2.2.2.2:8080"
	serverAddr := "1.1.1.1:8080"
	t.Run("with keep-alives", func(t *testing.T) {
		testHTTPMonitor(t, targetAddr, serverAddr, 100, testutil.Options{
			EnableKeepAlives: true,
		})
	})
	t.Run("without keep-alives", func(t *testing.T) {
		testHTTPMonitor(t, targetAddr, serverAddr, 100, testutil.Options{
			EnableKeepAlives: false,
		})
	})
}

func TestUnknownMethodRegression(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < kernel.VersionCode(4, 1, 0) {
		t.Skip("HTTP feature not available on pre 4.1.0 kernels")
	}

	// SetupDNAT sets up a NAT translation from 2.2.2.2 to 1.1.1.1
	netlink.SetupDNAT(t)

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

func TestRSTPacketRegression(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < kernel.VersionCode(4, 1, 0) {
		t.Skip("HTTP feature not available on pre 4.1.0 kernels")
	}

	monitor, err := NewMonitor(config.New(), nil, nil)
	require.NoError(t, err)
	err = monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()

	serverAddr := "127.0.0.1:8080"
	srvDoneFn := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableKeepAlives: true,
	})
	defer srvDoneFn()

	// Create a "raw" TCP socket that will serve as our HTTP client
	// We do this in order to configure the socket option SO_LINGER
	// so we can force a RST packet to be sent during termination
	c, err := net.DialTimeout("tcp", serverAddr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// Issue HTTP request
	c.Write([]byte("GET /200/foobar HTTP/1.1\nHost: 127.0.0.1:8080\n\n"))
	io.Copy(io.Discard, c)

	// Configure SO_LINGER to 0 so that triggers an RST when the socket is terminated
	require.NoError(t, c.(*net.TCPConn).SetLinger(0))
	c.Close()
	time.Sleep(100 * time.Millisecond)

	// Assert that the HTTP request was correctly handled despite its forceful termination
	stats := monitor.GetHTTPStats()
	url, err := url.Parse("http://127.0.0.1:8080/200/foobar")
	require.NoError(t, err)
	includesRequest(t, stats, &nethttp.Request{URL: url})
}

func testHTTPMonitor(t *testing.T, targetAddr, serverAddr string, numReqs int, o testutil.Options) {
	srvDoneFn := testutil.HTTPServer(t, serverAddr, o)

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
	srvDoneFn()

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
		if key.Path.Content == req.URL.Path && stats.HasStats(expectedStatus) {
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

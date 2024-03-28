// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client/fixtures"
)

func TestNewClientParams(t *testing.T) {
	tests := []struct {
		name          string
		endpoint      string
		username      string
		password      string
		expectedError string
	}{
		{
			name:          "all good",
			endpoint:      "test",
			username:      "testuser",
			password:      "testpassword",
			expectedError: "",
		},
		{
			name:          "no endpoint",
			endpoint:      "",
			username:      "testuser",
			password:      "testpassword",
			expectedError: "invalid endpoint",
		},
		{
			name:          "no username",
			endpoint:      "test",
			username:      "",
			password:      "testpassword",
			expectedError: "invalid username",
		},
		{
			name:          "no password",
			endpoint:      "test",
			username:      "testuser",
			password:      "",
			expectedError: "invalid password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(tt.endpoint, tt.username, tt.password, false)
			if tt.expectedError != "" {
				require.ErrorContains(t, err, tt.expectedError)
			}
		})
	}
}

func TestNewClientDefaults(t *testing.T) {
	tests := []struct {
		name                string
		options             []ClientOptions
		expectedMaxAttempts int
		expectedMaxPages    int
		expectedMaxCount    string
		expectedLookback    time.Duration
	}{
		{
			name:                "test defaults",
			options:             []ClientOptions{},
			expectedMaxAttempts: defaultMaxAttempts,
			expectedMaxCount:    defaultMaxCount,
			expectedMaxPages:    defaultMaxPages,
			expectedLookback:    defaultLookback,
		},
		{
			name: "No retries",
			options: []ClientOptions{
				WithMaxAttempts(1),
			},
			expectedMaxAttempts: 1,
			expectedMaxCount:    defaultMaxCount,
			expectedMaxPages:    defaultMaxPages,
			expectedLookback:    defaultLookback,
		},
		{
			name: "Count",
			options: []ClientOptions{
				WithMaxCount(4000),
			},
			expectedMaxAttempts: defaultMaxAttempts,
			expectedMaxCount:    "4000",
			expectedMaxPages:    defaultMaxPages,
			expectedLookback:    defaultLookback,
		},
		{
			name: "Pages",
			options: []ClientOptions{
				WithMaxPages(40),
			},
			expectedMaxAttempts: defaultMaxAttempts,
			expectedMaxCount:    defaultMaxCount,
			expectedMaxPages:    40,
			expectedLookback:    defaultLookback,
		},
		{
			name: "Lookback",
			options: []ClientOptions{
				WithLookback(40 * time.Minute),
			},
			expectedMaxAttempts: defaultMaxAttempts,
			expectedMaxCount:    defaultMaxCount,
			expectedMaxPages:    defaultMaxPages,
			expectedLookback:    40 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient("test", "testuser", "testpass", false, tt.options...)
			require.NoError(t, err)

			require.Equal(t, tt.expectedMaxAttempts, client.maxAttempts)
			require.Equal(t, tt.expectedMaxCount, client.maxCount)
			require.Equal(t, tt.expectedMaxPages, client.maxPages)
			require.Equal(t, tt.expectedLookback, client.lookback)
		})
	}
}

func TestGetDevices(t *testing.T) {
	mux, handler := setupCommonServerMuxWithFixture("/dataservice/device", fixtures.FakePayload(fixtures.GetDevices))

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	devices, err := client.GetDevices()
	require.NoError(t, err)

	require.Equal(t, "10.10.1.1", devices[0].DeviceID)
	require.Equal(t, "10.10.1.1", devices[0].SystemIP)
	require.Equal(t, "Manager", devices[0].HostName)
	require.Equal(t, "101", devices[0].SiteID)
	require.Equal(t, "reachable", devices[0].Reachability)
	require.Equal(t, "vmanage", devices[0].DeviceModel)
	require.Equal(t, "next", devices[0].DeviceOs)
	require.Equal(t, "20.12.1", devices[0].Version)
	require.Equal(t, "61FA4073B0169C46F4F498B8CA2C5C7A4A5510F9", devices[0].BoardSerial)

	// Ensure endpoint has been called 1 times
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestGetDevicesCounters(t *testing.T) {
	mux, handler := setupCommonServerMuxWithFixture("/dataservice/device/counters", fixtures.FakePayload(fixtures.GetDevicesCounters))

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	devices, err := client.GetDevicesCounters()
	require.NoError(t, err)

	require.Equal(t, "10.10.1.12", devices[0].SystemIP)
	require.Equal(t, 1, devices[0].NumberVsmartControlConnections)
	require.Equal(t, 1, devices[0].ExpectedControlConnections)
	require.Equal(t, 0, devices[0].OmpPeersUp)
	require.Equal(t, 0, devices[0].OmpPeersDown)
	require.Equal(t, 0, devices[0].BfdSessionsUp)
	require.Equal(t, 0, devices[0].BfdSessionsDown)
	require.Equal(t, 0, devices[0].CrashCount)
	require.Equal(t, 3, devices[0].RebootCount)

	// Ensure endpoint has been called 1 times
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestGetVEdgeInterfaces(t *testing.T) {
	mux := setupCommonServerMux()

	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		query := r.URL.Query()
		count := query.Get("count")

		require.Equal(t, "2000", count)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fixtures.FakePayload(fixtures.GetVEdgeInterfaces)))
	})

	mux.HandleFunc("/dataservice/data/device/state/Interface", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	devices, err := client.GetVEdgeInterfaces()
	require.NoError(t, err)

	require.Equal(t, "10.10.1.5", devices[0].VmanageSystemIP)
	require.Equal(t, "system", devices[0].Ifname)
	require.Equal(t, float64(3), devices[0].Ifindex)
	require.Equal(t, "", devices[0].Desc)
	require.Equal(t, "", devices[0].Hwaddr)
	require.Equal(t, "Up", devices[0].IfAdminStatus)
	require.Equal(t, "Up", devices[0].IfOperStatus)

	// Ensure endpoint has been called 1 times
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestGetCEdgeInterfaces(t *testing.T) {
	mux := setupCommonServerMux()

	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		query := r.URL.Query()
		count := query.Get("count")

		require.Equal(t, "2000", count)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fixtures.FakePayload(fixtures.GetCEdgeInterfaces)))
	})

	mux.HandleFunc("/dataservice/data/device/state/CEdgeInterface", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	devices, err := client.GetCEdgeInterfaces()
	require.NoError(t, err)

	require.Equal(t, "10.10.1.17", devices[0].VmanageSystemIP)
	require.Equal(t, "GigabitEthernet4", devices[0].Ifname)
	require.Equal(t, "4", devices[0].Ifindex)
	require.Equal(t, "", devices[0].Description)
	require.Equal(t, "52:54:00:0b:6e:90", devices[0].Hwaddr)
	require.Equal(t, "if-state-up", devices[0].IfAdminStatus)
	require.Equal(t, "if-oper-state-ready", devices[0].IfOperStatus)

	// Ensure endpoint has been called 1 times
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestGetInterfacesMetrics(t *testing.T) {
	timeNow = mockTimeNow
	mux := setupCommonServerMux()

	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		query := r.URL.Query()
		count := query.Get("count")
		timeZone := query.Get("timeZone")
		startDate := query.Get("startDate")
		endDate := query.Get("endDate")

		require.Equal(t, "2000", count)
		require.Equal(t, "UTC", timeZone)
		require.Equal(t, "1999-12-31T23:40:00", startDate)
		require.Equal(t, "2000-01-01T00:00:00", endDate)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fixtures.FakePayload(fixtures.GetInterfacesMetrics)))
	})

	mux.HandleFunc("/dataservice/data/device/statistics/interfacestatistics", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	devices, err := client.GetInterfacesMetrics()
	require.NoError(t, err)

	require.Equal(t, "10.10.1.22", devices[0].VmanageSystemIP)
	require.Equal(t, "GigabitEthernet3", devices[0].Interface)
	require.Equal(t, float64(1709049697985), devices[0].EntryTime)
	require.Equal(t, float64(4), devices[0].TxOctets)
	require.Equal(t, float64(23), devices[0].RxOctets)
	require.Equal(t, 9.8, devices[0].TxKbps)
	require.Equal(t, 10.4, devices[0].RxKbps)
	require.Equal(t, float64(0), devices[0].DownCapacityPercentage)
	require.Equal(t, 0.8, devices[0].UpCapacityPercentage)
	require.Equal(t, float64(2), devices[0].RxErrors)
	require.Equal(t, float64(506), devices[0].TxErrors)
	require.Equal(t, float64(6), devices[0].RxDrops)
	require.Equal(t, float64(3), devices[0].TxDrops)

	// Ensure endpoint has been called 1 times
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestGetDeviceHardwareMetrics(t *testing.T) {
	timeNow = mockTimeNow
	mux := setupCommonServerMux()

	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		query := r.URL.Query()
		count := query.Get("count")
		timeZone := query.Get("timeZone")
		startDate := query.Get("startDate")
		endDate := query.Get("endDate")

		require.Equal(t, "2000", count)
		require.Equal(t, "UTC", timeZone)
		require.Equal(t, "1999-12-31T23:40:00", startDate)
		require.Equal(t, "2000-01-01T00:00:00", endDate)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fixtures.FakePayload(fixtures.GetDeviceHardwareStatistics)))
	})

	mux.HandleFunc("/dataservice/data/device/statistics/devicesystemstatusstatistics", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	devices, err := client.GetDeviceHardwareMetrics()
	require.NoError(t, err)

	require.Equal(t, "10.10.1.5", devices[0].VmanageSystemIP)
	require.Equal(t, float64(1709050342874), devices[0].EntryTime)
	require.Equal(t, 0.29, devices[0].CPUUserNew)
	require.Equal(t, 0.41, devices[0].CPUSystemNew)
	require.Equal(t, 0.15, devices[0].MemUtil)
	require.Equal(t, float64(293187584), devices[0].DiskUsed)
	require.Equal(t, float64(7245897728), devices[0].DiskAvail)

	// Ensure endpoint has been called 1 times
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestGetApplicationAwareRoutingMetrics(t *testing.T) {
	timeNow = mockTimeNow
	mux := setupCommonServerMux()

	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		query := r.URL.Query()
		count := query.Get("count")
		timeZone := query.Get("timeZone")
		startDate := query.Get("startDate")
		endDate := query.Get("endDate")

		require.Equal(t, "2000", count)
		require.Equal(t, "UTC", timeZone)
		require.Equal(t, "1999-12-31T23:40:00", startDate)
		require.Equal(t, "2000-01-01T00:00:00", endDate)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fixtures.FakePayload(fixtures.GetApplicationAwareRoutingMetrics)))
	})

	mux.HandleFunc("/dataservice/data/device/statistics/approutestatsstatistics", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	devices, err := client.GetApplicationAwareRoutingMetrics()
	require.NoError(t, err)

	require.Equal(t, "10.10.1.13", devices[0].VmanageSystemIP)
	require.Equal(t, "10.10.1.13", devices[0].LocalSystemIP)
	require.Equal(t, "10.10.1.11", devices[0].RemoteSystemIP)
	require.Equal(t, "mpls", devices[0].LocalColor)
	require.Equal(t, "public-internet", devices[0].RemoteColor)
	require.Equal(t, float64(1709050725125), devices[0].EntryTime)
	require.Equal(t, float64(202), devices[0].Latency)
	require.Equal(t, float64(0), devices[0].Jitter)
	require.Equal(t, 0.301, devices[0].LossPercentage)
	require.Equal(t, float64(2), devices[0].VqoeScore)

	// Ensure endpoint has been called 1 times
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestGetControlConnectionsState(t *testing.T) {
	mux := setupCommonServerMux()

	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		query := r.URL.Query()
		count := query.Get("count")

		require.Equal(t, "2000", count)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fixtures.FakePayload(fixtures.GetControlConnectionsState)))
	})

	mux.HandleFunc("/dataservice/data/device/state/ControlConnection", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	devices, err := client.GetControlConnectionsState()
	require.NoError(t, err)

	require.Equal(t, "10.10.1.1", devices[0].VmanageSystemIP)
	require.Equal(t, "10.10.1.3", devices[0].SystemIP)
	require.Equal(t, "10.10.20.80", devices[0].PrivateIP)
	require.Equal(t, "default", devices[0].LocalColor)
	require.Equal(t, "default", devices[0].RemoteColor)
	require.Equal(t, "vbond", devices[0].PeerType)
	require.Equal(t, "up", devices[0].State)

	// Ensure endpoint has been called 1 times
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestGetOMPPeersState(t *testing.T) {
	mux := setupCommonServerMux()

	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		query := r.URL.Query()
		count := query.Get("count")

		require.Equal(t, "2000", count)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fixtures.FakePayload(fixtures.GetOMPPeersState)))
	})

	mux.HandleFunc("/dataservice/data/device/state/OMPPeer", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	devices, err := client.GetOMPPeersState()
	require.NoError(t, err)

	require.Equal(t, "10.10.1.5", devices[0].VmanageSystemIP)
	require.Equal(t, "10.10.1.13", devices[0].Peer)
	require.Equal(t, "yes", devices[0].Legit)
	require.Equal(t, "supported", devices[0].Refresh)
	require.Equal(t, "vedge", devices[0].Type)
	require.Equal(t, "up", devices[0].State)

	// Ensure endpoint has been called 1 times
	require.Equal(t, 1, handler.numberOfCalls())
}

func TestGetBFDSessionsState(t *testing.T) {
	mux := setupCommonServerMux()

	handler := newHandler(func(w http.ResponseWriter, r *http.Request, calls int32) {
		query := r.URL.Query()
		count := query.Get("count")

		require.Equal(t, "2000", count)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fixtures.FakePayload(fixtures.GetBFDSessionsState)))
	})

	mux.HandleFunc("/dataservice/data/device/state/BFDSessions", handler.Func)

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := testClient(server)
	require.NoError(t, err)

	devices, err := client.GetBFDSessionsState()
	require.NoError(t, err)

	require.Equal(t, "10.10.1.11", devices[0].VmanageSystemIP)
	require.Equal(t, "public-internet", devices[0].LocalColor)
	require.Equal(t, "public-internet", devices[0].Color)
	require.Equal(t, "10.10.1.13", devices[0].SystemIP)
	require.Equal(t, "ipsec", devices[0].Proto)
	require.Equal(t, "up", devices[0].State)

	// Ensure endpoint has been called 1 times
	require.Equal(t, 1, handler.numberOfCalls())
}

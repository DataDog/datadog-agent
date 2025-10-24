// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	devicepluginv1beta1 "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

func TestSingleDevicePluginClient_NewSingleDevicePluginClientWithSocket(t *testing.T) {
	socketPath := runMockDevicePluginServer(t, "", []*devicepluginv1beta1.Device{})
	client, err := NewSingleDevicePluginClientWithSocket(socketPath)
	require.NoError(t, err)
	require.NotNil(t, client)
	defer require.NotPanics(t, client.Close)

	assert.NotNil(t, client.conn)
	assert.NotNil(t, client.client)
}

func TestSingleDevicePluginClient_ListDevices(t *testing.T) {
	mockDevices := []*devicepluginv1beta1.Device{
		{
			ID:     "gpu-0",
			Health: devicepluginv1beta1.Healthy,
		},
		{
			ID:     "gpu-1",
			Health: devicepluginv1beta1.Unhealthy,
		},
		{
			ID:     "gpu-2",
			Health: devicepluginv1beta1.Healthy,
		},
	}

	socketPath := runMockDevicePluginServer(t, "", mockDevices)
	client, err := NewSingleDevicePluginClientWithSocket(socketPath)
	require.NoError(t, err)
	defer client.Close()

	devices, err := client.ListDevices(t.Context())
	require.NoError(t, err)
	require.Len(t, devices, 3)
	assert.Equal(t, mockDevices[0].ID, devices[0].ID)
	assert.Equal(t, devicepluginv1beta1.Healthy, devices[0].Health)
	assert.Equal(t, mockDevices[1].ID, devices[1].ID)
	assert.Equal(t, devicepluginv1beta1.Unhealthy, devices[1].Health)
	assert.Equal(t, mockDevices[2].ID, devices[2].ID)
	assert.Equal(t, devicepluginv1beta1.Healthy, devices[2].Health)
}

func TestMultiDevicePluginClient(t *testing.T) {
	tmpDir := t.TempDir()

	// No client should be available yet since firstRefresh=false
	client, err := NewMultiDevicePluginClientWithSocketDir(tmpDir, false)
	require.NoError(t, err)
	require.NotNil(t, client)
	defer client.Close()
	assert.Equal(t, tmpDir, client.socketDir)
	assert.NotNil(t, client.socketToClients)
	assert.Len(t, client.socketToClients, 0)

	// Start mock gRPC servers
	socket1 := filepath.Join(tmpDir, "plugin1.sock")
	socket2 := filepath.Join(tmpDir, "plugin2.sock")
	runMockDevicePluginServer(t, socket1, []*devicepluginv1beta1.Device{{ID: "device1", Health: devicepluginv1beta1.Healthy}})
	_, cleanup2 := runMockDevicePluginServerWithCleanup(t, socket2, []*devicepluginv1beta1.Device{{ID: "device2", Health: devicepluginv1beta1.Healthy}})
	t.Cleanup(cleanup2)

	// Create other files that should be ignored
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "kubelet.sock"), []byte{}, os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte{}, os.ModePerm))

	// Trigger a refresh in the client and makes sure it scrapes sockets right
	err = client.Refresh(t.Context())
	require.NoError(t, err)
	client.mu.RLock()
	assert.Len(t, client.socketToClients, 2)
	client.mu.RUnlock()

	// Try to list devices, and make sure all are present
	devices, err := client.ListDevices(t.Context())
	require.NoError(t, err)
	require.Len(t, devices, 2)
	deviceIDs := map[string]struct{}{}
	for _, device := range devices {
		deviceIDs[device.ID] = struct{}{}
	}
	assert.Contains(t, deviceIDs, "device1")
	assert.Contains(t, deviceIDs, "device2")

	// Stop one of the two server, and make sure socket disappears and refresh leads to a reconciliation
	cleanup2()
	assert.EventuallyWithT(t, func(t *assert.CollectT) {
		_, err := os.Stat(socket2)
		assert.ErrorIs(t, err, os.ErrNotExist)
	}, 2*time.Second, 200*time.Millisecond)

	err = client.Refresh(t.Context())
	require.NoError(t, err)
	client.mu.RLock()
	assert.Len(t, client.socketToClients, 1)
	client.mu.RUnlock()

	// List devices again, this time only the first one should appear
	devices, err = client.ListDevices(t.Context())
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Contains(t, deviceIDs, "device1")
}

func TestCachedDevicePluginClient(t *testing.T) {
	mockClient := &mockDevicePluginClient{
		devices: []*devicepluginv1beta1.Device{{ID: "device1", Health: devicepluginv1beta1.Healthy}},
	}

	cacheTimeout := 1 * time.Second
	cachedClient, err := NewCachedDevicePluginClient(mockClient, cacheTimeout)
	require.NoError(t, err)
	defer cachedClient.Close()

	// First call should hit the underlying client
	require.NoError(t, cachedClient.Refresh(t.Context()))
	devices, err := cachedClient.ListDevices(t.Context())
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, "device1", devices[0].ID)

	// Second call within timeout should return cached result
	lastRefreshTime := mockClient.lastRefreshTime
	lastListDevicesTime := mockClient.lastListDevicesTime
	mockClient.devices = append(mockClient.devices, &devicepluginv1beta1.Device{ID: "device2", Health: devicepluginv1beta1.Healthy})

	require.NoError(t, cachedClient.Refresh(t.Context()))
	devices, err = cachedClient.ListDevices(t.Context())
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, "device1", devices[0].ID)
	assert.Equal(t, lastListDevicesTime, mockClient.lastListDevicesTime)
	assert.Equal(t, lastRefreshTime, mockClient.lastRefreshTime)

	// After timeout, should hit the underlying client again
	time.Sleep(cacheTimeout)
	lastRefreshTime = mockClient.lastRefreshTime
	lastListDevicesTime = mockClient.lastListDevicesTime

	require.NoError(t, cachedClient.Refresh(t.Context()))
	devices, err = cachedClient.ListDevices(t.Context())
	require.NoError(t, err)
	require.Len(t, devices, 2)
	assert.Equal(t, "device1", devices[0].ID)
	assert.Equal(t, "device2", devices[1].ID)
	assert.Less(t, lastListDevicesTime, mockClient.lastListDevicesTime)
	assert.Less(t, lastRefreshTime, mockClient.lastRefreshTime)
}

type mockDevicePluginServer struct {
	devicepluginv1beta1.UnimplementedDevicePluginServer
	devices []*devicepluginv1beta1.Device
}

func (m *mockDevicePluginServer) ListAndWatch(_ *devicepluginv1beta1.Empty, stream devicepluginv1beta1.DevicePlugin_ListAndWatchServer) error {
	return stream.Send(&devicepluginv1beta1.ListAndWatchResponse{Devices: m.devices})
}

func runMockDevicePluginServer(t *testing.T, socketPath string, devices []*devicepluginv1beta1.Device) string {
	socket, cleanup := runMockDevicePluginServerWithCleanup(t, socketPath, devices)
	t.Cleanup(cleanup)
	return socket
}

func runMockDevicePluginServerWithCleanup(t *testing.T, socketPath string, devices []*devicepluginv1beta1.Device) (string, func()) {
	tmpDir := t.TempDir()
	if socketPath == "" {
		socketPath = filepath.Join(tmpDir, "nvidia-gpu-mock.sock")
	}

	os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	server := grpc.NewServer()
	mockServer := &mockDevicePluginServer{devices: devices}
	devicepluginv1beta1.RegisterDevicePluginServer(server, mockServer)

	// Give the server a moment to start
	unixSocketPath := "unix://" + socketPath
	go func() { assert.NoError(t, server.Serve(listener)) }()
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		conn, err := grpc.NewClient(unixSocketPath, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions())
		require.NoError(t, err)
		require.NotNil(t, conn)
		require.NoError(t, conn.Close())
	}, 5*time.Second, 200*time.Millisecond)

	stopped := false
	stoppedMtx := sync.Mutex{}
	return unixSocketPath, func() {
		stoppedMtx.Lock()
		defer stoppedMtx.Unlock()
		if !stopped {
			server.Stop()
			listener.Close()
			os.Remove(socketPath)
			stopped = true
		}
	}
}

type mockDevicePluginClient struct {
	lastRefreshTime     time.Time
	lastListDevicesTime time.Time
	err                 error
	devices             []*devicepluginv1beta1.Device
}

func (m *mockDevicePluginClient) Close() {}

func (m *mockDevicePluginClient) Refresh(_ context.Context) error {
	m.lastRefreshTime = time.Now()
	return m.err
}

func (m *mockDevicePluginClient) ListDevices(_ context.Context) ([]*devicepluginv1beta1.Device, error) {
	m.lastListDevicesTime = time.Now()
	return m.devices, m.err
}

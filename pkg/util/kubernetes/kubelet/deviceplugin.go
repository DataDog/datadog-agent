// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	devicepluginv1beta1 "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DevicePluginClient retrieves health/metadata about the available devices
type DevicePluginClient interface {
	// Close closes the client
	Close()
	// Refresh is a generic method for updating internal info/resources in the client
	Refresh(ctx context.Context) error
	// ListDevices returns the most up to date list of devices across all the device plugins
	ListDevices(ctx context.Context) ([]*devicepluginv1beta1.Device, error)
}

// NewDevicePluginClient creates a new DevicePluginClient from the configuration. Will fail if the socket dir is not set.
func NewDevicePluginClient(config config.Component) (DevicePluginClient, error) {
	socketDir := config.GetString("kubernetes_kubelet_deviceplugins_socketdir")
	if socketDir == "" {
		return nil, errors.New("kubernetes_kubelet_deviceplugins_socketdir is not set")
	}

	multiClient, err := NewMultiDevicePluginClientWithSocketDir(socketDir, true)
	if err != nil {
		return nil, fmt.Errorf("failed creating multi device client: %w", err)
	}

	cacheDuration := config.GetDuration("kubernetes_kubelet_deviceplugins_cache_duration")
	if cacheDuration == 0 {
		log.Warn("kubernetes_kubelet_deviceplugins_cache_duration is not set, will use uncached kubelet device plugins client")
		return multiClient, nil
	}

	cachedClient, err := NewCachedDevicePluginClient(multiClient, cacheDuration)
	if err != nil {
		return nil, fmt.Errorf("failed creating plugin client cache: %w", err)
	}

	return cachedClient, nil
}

// SingleDevicePluginClient is a small wrapper for the DevicePlugin kubernetes API
type SingleDevicePluginClient struct {
	conn   *grpc.ClientConn
	client devicepluginv1beta1.DevicePluginClient
}

// NewSingleDevicePluginClientWithSocket creates a new DevicePluginClient using the
// provided socket path (must start with unix:// on Linux or npipe:// on Windows).
func NewSingleDevicePluginClientWithSocket(socket string) (*SingleDevicePluginClient, error) {
	socketPrefix := "unix://"
	if runtime.GOOS == "windows" {
		socketPrefix = "npipe://"
	}
	if !strings.HasPrefix(socket, socketPrefix) {
		socket = socketPrefix + socket
	}

	conn, err := grpc.NewClient(
		socket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)),
	)
	if err != nil {
		return nil, fmt.Errorf("failure creating gRPC client: %w", err)
	}

	return &SingleDevicePluginClient{
		conn:   conn,
		client: devicepluginv1beta1.NewDevicePluginClient(conn),
	}, nil
}

// Close closes the connection to the gRPC server
func (c *SingleDevicePluginClient) Close() {
	c.conn.Close()
}

// Refresh does nothing for single plugin clients
func (c *SingleDevicePluginClient) Refresh(_ context.Context) error {
	return nil
}

// ListDevices returns the most up to date list of devices from the plugin's gRPC server
func (c *SingleDevicePluginClient) ListDevices(ctx context.Context) ([]*devicepluginv1beta1.Device, error) {
	// cancelling the context will close the watch client below
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// note: this is supposed to be a stream API but we have no interest in maintaining a long-lived
	// connection to the socket, so we just pull and close every time. When invoking this, the first
	// Recv() will return the device list
	stream, err := c.client.ListAndWatch(ctx, &devicepluginv1beta1.Empty{})
	if err != nil {
		return nil, fmt.Errorf("failed invoking ListAndWatch: %w", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed receiving device list: %w", err)
	}

	return resp.Devices, nil
}

// MultiDevicePluginClient is an orchestrator of multiple DevicePluginClients,
// each specialized on the socket of one specific device plugin
type MultiDevicePluginClient struct {
	mu              sync.RWMutex // mutex to protect the socketToClients map
	socketDir       string
	socketToClients map[string]*SingleDevicePluginClient
}

// NewMultiDevicePluginClientWithSocketDir returns a client that scrapes through the
// device plugins socket dir (e.g. /var/lib/kubelet/device-plugins) for valid plugin sockets
// and opens a client for each of them. When listing devices, it takes care of pulling
// information from all the available sockets.
func NewMultiDevicePluginClientWithSocketDir(socketDir string, firstRefresh bool) (*MultiDevicePluginClient, error) {
	client := &MultiDevicePluginClient{
		socketDir:       socketDir,
		socketToClients: map[string]*SingleDevicePluginClient{},
	}

	if !firstRefresh {
		return client, nil
	}

	// don't wait indefinitely in case of unexpected hangs
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Refresh(ctx); err != nil {
		return nil, err
	}

	return client, nil
}

// Close closes the client and all the underlying clients to the device plugin sockets
func (c *MultiDevicePluginClient) Close() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, client := range c.socketToClients {
		client.Close()
	}
}

// Refresh checks the current list of device plugins sockets and opens/closes clients depending
// on any differences from the previous known list of available plugin sockets
func (c *MultiDevicePluginClient) Refresh(_ context.Context) error {
	entries, err := os.ReadDir(c.socketDir)
	if err != nil {
		return fmt.Errorf("failed listing sockets in %s: %w", c.socketDir, err)
	}

	// get an updated list of device plugin sockets
	pluginSockets := map[string]struct{}{}
	for _, entry := range entries {
		name := entry.Name()

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("failed getting socket info %s: %w", name, err)
		}

		if info.Mode()&os.ModeSocket == 0 ||
			strings.HasPrefix(name, "kubelet") ||
			(!strings.HasSuffix(name, ".sock") && !strings.HasSuffix(name, ".pipe")) {
			continue
		}

		socketPath, err := c.sanitizeRootFsPath(c.socketDir, name)
		if err != nil {
			return fmt.Errorf("failed getting absolute path for %s/%s: %w", c.socketDir, name, err)
		}

		pluginSockets[socketPath] = struct{}{}
	}

	// lock from this point onwards to protect c.socketToClients
	c.mu.Lock()
	defer c.mu.Unlock()

	// compare with the current list of clients and determine which needs to be opened/closed
	toOpen := []string{}
	toClose := []string{}
	for socket := range c.socketToClients {
		if _, ok := pluginSockets[socket]; !ok {
			toClose = append(toClose, socket)
		}
	}
	for socket := range pluginSockets {
		if _, ok := c.socketToClients[socket]; !ok {
			toOpen = append(toOpen, socket)
		}
	}

	// close clients for sockets that are no more available
	for _, socket := range toClose {
		c.socketToClients[socket].Close()
		delete(c.socketToClients, socket)
	}

	// open clients for new sockets
	for _, socket := range toOpen {
		client, err := NewSingleDevicePluginClientWithSocket(socket)
		if err != nil {
			return fmt.Errorf("failed opening client for %s: %w", socket, err)
		}
		c.socketToClients[socket] = client
	}

	return nil
}

func (c *MultiDevicePluginClient) sanitizeRootFsPath(basedir string, parts ...string) (string, error) {
	path := filepath.Join(append([]string{basedir}, parts...)...)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path %s, could not get absolute path: %w", path, err)
	}
	if !strings.HasPrefix(absPath, basedir) {
		return "", fmt.Errorf("invalid path %s, should be a child of %s", absPath, basedir)
	}
	return absPath, nil
}

// ListDevices returns the most up to date list of devices from all the device plugin sockets available
func (c *MultiDevicePluginClient) ListDevices(ctx context.Context) ([]*devicepluginv1beta1.Device, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	allDevices := []*devicepluginv1beta1.Device{}
	for socket, client := range c.socketToClients {
		devices, err := client.ListDevices(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed listing devices for %s: %w", socket, err)
		}
		allDevices = append(allDevices, devices...)
	}
	return allDevices, nil
}

// CachedDevicePluginClient caches the method invocations of another DevicePluginClient
type CachedDevicePluginClient struct {
	mu              sync.Mutex // mutex to serialize access to the client and lastDevices map
	client          DevicePluginClient
	timeout         time.Duration
	lastDevices     []*devicepluginv1beta1.Device
	lastDevicesTime time.Time
	lastRefreshTime time.Time
}

// NewCachedDevicePluginClient returns a client that caches method invocations of another client
// to reduce usage/pressure on its resources
func NewCachedDevicePluginClient(client DevicePluginClient, timeout time.Duration) (*CachedDevicePluginClient, error) {
	return &CachedDevicePluginClient{client: client, timeout: timeout}, nil
}

// Close closes the client and all the underlying clients to the device plugin sockets
func (c *CachedDevicePluginClient) Close() {
	c.client.Close()
}

// Refresh invokes Refresh on the underlying client according to the given cache timeout
func (c *CachedDevicePluginClient) Refresh(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Since(c.lastRefreshTime) > c.timeout {
		if err := c.client.Refresh(ctx); err != nil {
			return err
		}
		c.lastRefreshTime = time.Now()
	}
	return nil
}

// ListDevices invokes ListDevices on the underlying client and caches the result. Un until the
// cache is valid, it will keep returning the cached list. The result is not cached in case of error.
func (c *CachedDevicePluginClient) ListDevices(ctx context.Context) ([]*devicepluginv1beta1.Device, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Since(c.lastDevicesTime) > c.timeout {
		devices, err := c.client.ListDevices(ctx)
		if err != nil {
			return nil, err
		}
		c.lastDevices = devices
		c.lastDevicesTime = time.Now()
	}
	return c.lastDevices, nil
}

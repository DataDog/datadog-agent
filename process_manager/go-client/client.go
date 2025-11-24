// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package procmgr provides a Go client for the DataDog Process Manager
package procmgr

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client is the Process Manager gRPC client
type Client struct {
	conn   *grpc.ClientConn
	client ProcessManagerClient
}

// clientConfig holds client configuration
type clientConfig struct {
	unixSocket string
	tcpAddress string
}

// ClientOption configures the client
type ClientOption func(*clientConfig)

// WithUnixSocket configures the client to use a Unix socket
func WithUnixSocket(path string) ClientOption {
	return func(c *clientConfig) {
		c.unixSocket = path
		c.tcpAddress = ""
	}
}

// WithTCP configures the client to use TCP
func WithTCP(address string) ClientOption {
	return func(c *clientConfig) {
		c.tcpAddress = address
		c.unixSocket = ""
	}
}

// NewClientWithAddress creates a new client connected to the specified address
// For TCP: "localhost:50051"
// For Unix socket: use NewClient() with WithUnixSocket() option instead
func NewClientWithAddress(address string) (*Client, error) {
	return NewClient(WithTCP(address))
}

// NewClient creates a new Process Manager client
// Default: Unix socket at /var/run/process-manager.sock
func NewClient(opts ...ClientOption) (*Client, error) {
	config := &clientConfig{
		unixSocket: "/var/run/process-manager.sock",
	}

	for _, opt := range opts {
		opt(config)
	}

	var target string
	var dialOpts []grpc.DialOption

	if config.unixSocket != "" {
		// Unix socket connection
		target = "unix://" + config.unixSocket
		dialOpts = append(dialOpts,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
				return net.Dial("unix", config.unixSocket)
			}),
		)
	} else {
		// TCP connection
		target = config.tcpAddress
		dialOpts = append(dialOpts,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, target, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to process manager: %w", err)
	}

	return &Client{
		conn:   conn,
		client: NewProcessManagerClient(conn),
	}, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// CreateProcess creates a new process
func (c *Client) CreateProcess(ctx context.Context, req *CreateRequest) (*CreateResponse, error) {
	return c.client.Create(ctx, req)
}

// StartProcess starts a process by ID
func (c *Client) StartProcess(ctx context.Context, id string) error {
	_, err := c.client.Start(ctx, &StartRequest{
		Id: id,
	})
	return err
}

// StopProcess stops a process by ID
func (c *Client) StopProcess(ctx context.Context, id string) error {
	_, err := c.client.Stop(ctx, &StopRequest{
		Id: id,
	})
	return err
}

// DeleteProcess deletes a process by ID
func (c *Client) DeleteProcess(ctx context.Context, id string, force bool) error {
	_, err := c.client.Delete(ctx, &DeleteRequest{
		Id:    id,
		Force: force,
	})
	return err
}

// ListProcesses lists all processes
func (c *Client) ListProcesses(ctx context.Context) ([]*Process, error) {
	resp, err := c.client.List(ctx, &ListRequest{})
	if err != nil {
		return nil, err
	}
	return resp.Processes, nil
}

// DescribeProcess gets detailed information about a process
func (c *Client) DescribeProcess(ctx context.Context, id string) (*ProcessDetail, error) {
	resp, err := c.client.Describe(ctx, &DescribeRequest{
		Id: id,
	})
	if err != nil {
		return nil, err
	}
	return resp.Detail, nil
}

// UpdateProcess updates a process configuration
func (c *Client) UpdateProcess(ctx context.Context, req *UpdateRequest) (*UpdateResponse, error) {
	return c.client.Update(ctx, req)
}

// GetResourceUsage gets resource usage statistics for a process
func (c *Client) GetResourceUsage(ctx context.Context, id string) (*GetResourceUsageResponse, error) {
	return c.client.GetResourceUsage(ctx, &GetResourceUsageRequest{
		Id: id,
	})
}

// GetStatus gets detailed daemon status
func (c *Client) GetStatus(ctx context.Context) (*GetStatusResponse, error) {
	return c.client.GetStatus(ctx, &GetStatusRequest{})
}

// ReloadConfig reloads the daemon configuration
func (c *Client) ReloadConfig(ctx context.Context) error {
	_, err := c.client.ReloadConfig(ctx, &ReloadConfigRequest{})
	return err
}

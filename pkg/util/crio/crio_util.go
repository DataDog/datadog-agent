// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package crio provides a crio client.
package crio

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	udsPrefix = "unix://%s"
)

// Client defines an interface for interacting with the CRI-API, providing methods for
// retrieving information about container and pod statuses, images, and metadata.
type Client interface {
	// Close closes the connection to the CRI-O API and cleans up resources.
	Close() error

	// RuntimeMetadata retrieves version information and metadata about the container runtime.
	// ctx: The context for managing the lifetime of the request.
	RuntimeMetadata(ctx context.Context) (*v1.VersionResponse, error)

	// GetAllContainers lists all containers managed by the CRI-O runtime.
	// ctx: The context for managing the lifetime of the request.
	GetAllContainers(ctx context.Context) ([]*v1.Container, error)

	// GetContainerStatus retrieves the current status of a specific container.
	// ctx: The context for managing the lifetime of the request.
	// containerID: The unique identifier of the container for which the status is requested.
	GetContainerStatus(ctx context.Context, containerID string) (*v1.ContainerStatusResponse, error)

	// GetContainerImage retrieves metadata and status information about a specific image.
	// ctx: The context for managing the lifetime of the request.
	// imageSpec: A reference to the image, which includes its name or other identifying information.
	GetContainerImage(ctx context.Context, imageSpec *v1.ImageSpec) (*v1.Image, error)

	// GetPodStatus retrieves the current status of a specific pod sandbox.
	// ctx: The context for managing the lifetime of the request.
	// podSandboxID: The unique identifier of the pod sandbox for which the status is requested.
	GetPodStatus(ctx context.Context, podSandboxID string) (*v1.PodSandboxStatus, error)
}

// clientImpl is a client to interact with the CRI-API.
type clientImpl struct {
	runtimeClient v1.RuntimeServiceClient
	imageClient   v1.ImageServiceClient
	conn          *grpc.ClientConn
	initRetry     retry.Retrier
	socketPath    string
}

// NewCRIOClient creates a new CRI-O client implementing the Client interface.
func NewCRIOClient(socketPath string) (Client, error) {

	client := &clientImpl{socketPath: socketPath}

	client.initRetry.SetupRetrier(&retry.Config{ //nolint:errcheck
		Name:              "crio-client",
		AttemptMethod:     client.connect,
		Strategy:          retry.Backoff,
		InitialRetryDelay: 1 * time.Second,
		MaxRetryDelay:     5 * time.Minute,
	})

	// Attempt connection with retry
	if err := client.initRetry.TriggerRetry(); err != nil {
		return nil, fmt.Errorf("failed to initialize CRI-O client: %w", err)
	}

	return client, nil
}

// Close closes the CRI-O client connection.
func (c *clientImpl) Close() error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("CRI-O client is not initialized")
	}
	return c.conn.Close()
}

// RuntimeMetadata retrieves the runtime metadata including runtime name and version.
func (c *clientImpl) RuntimeMetadata(ctx context.Context) (*v1.VersionResponse, error) {
	return c.runtimeClient.Version(ctx, &v1.VersionRequest{})
}

// GetAllContainers retrieves all containers.
func (c *clientImpl) GetAllContainers(ctx context.Context) ([]*v1.Container, error) {
	containersResponse, err := c.runtimeClient.ListContainers(ctx, &v1.ListContainersRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	return containersResponse.GetContainers(), nil
}

// GetContainerStatus retrieves the status of a specific container.
func (c *clientImpl) GetContainerStatus(ctx context.Context, containerID string) (*v1.ContainerStatusResponse, error) {
	containerStatusResponse, err := c.runtimeClient.ContainerStatus(ctx, &v1.ContainerStatusRequest{ContainerId: containerID, Verbose: true})
	if err != nil {
		return nil, fmt.Errorf("failed to get container status for ID %s: %w", containerID, err)
	}
	return containerStatusResponse, nil
}

// GetContainerImage retrieves the image status of a specific imageSpec.
func (c *clientImpl) GetContainerImage(ctx context.Context, imageSpec *v1.ImageSpec) (*v1.Image, error) {
	imageStatusResponse, err := c.imageClient.ImageStatus(ctx, &v1.ImageStatusRequest{Image: imageSpec})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image status for spec %s: %w", imageSpec.Image, err)
	}
	if imageStatusResponse.GetImage() == nil {
		return nil, fmt.Errorf("image not found for spec %s", imageSpec.Image)
	}
	return imageStatusResponse.GetImage(), nil
}

// GetPodStatus retrieves the status of a specific pod sandbox.
func (c *clientImpl) GetPodStatus(ctx context.Context, podSandboxID string) (*v1.PodSandboxStatus, error) {
	podSandboxStatusResponse, err := c.runtimeClient.PodSandboxStatus(ctx, &v1.PodSandboxStatusRequest{PodSandboxId: podSandboxID})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod status for pod ID %s: %w", podSandboxID, err)
	}
	return podSandboxStatusResponse.GetStatus(), nil
}

// connect establishes a gRPC connection.
func (c *clientImpl) connect() error {
	socketURI := fmt.Sprintf(udsPrefix, c.socketPath)
	conn, err := grpc.NewClient(socketURI, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to CRI-O socket at %s: %w", socketURI, err)
	}

	c.conn = conn
	c.runtimeClient = v1.NewRuntimeServiceClient(conn)
	c.imageClient = v1.NewImageServiceClient(conn)

	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer rpcCancel()

	// Make an initial request to ensure the connection is working and transition it to a ready state
	if _, err := c.RuntimeMetadata(rpcCtx); err != nil {
		conn.Close()
		return fmt.Errorf("initial RPC failed: %w", err)
	}

	// Ensure connection is ready
	if conn.GetState() != connectivity.Ready {
		return fmt.Errorf("connection not in READY state")
	}

	return nil
}

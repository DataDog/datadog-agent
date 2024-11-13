// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package crio provides a crio client.
package crio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	defaultCrioSocketPath = "/var/run/crio/crio.sock"
	udsPrefix             = "unix://%s"
	overlayPath           = "/var/lib/containers/storage/overlay"
	OverlayImagePath      = "/var/lib/containers/storage/overlay-images"
	overlayLayersPath     = "/var/lib/containers/storage/overlay-layers/layers.json"
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
	// verbose: If set to true, includes detailed metadata in the response.
	GetContainerImage(ctx context.Context, imageSpec *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error)

	// GetPodStatus retrieves the current status of a specific pod sandbox.
	// ctx: The context for managing the lifetime of the request.
	// podSandboxID: The unique identifier of the pod sandbox for which the status is requested.
	GetPodStatus(ctx context.Context, podSandboxID string) (*v1.PodSandboxStatus, error)

	// GetCRIOImageLayers returns the paths of the `diff` directories for each layer of the specified image.
	// imgMeta: Metadata information about the container image whose layer paths are requested.
	GetCRIOImageLayers(imgMeta *workloadmeta.ContainerImageMetadata) ([]string, error)
}

// clientImpl is a client to interact with the CRI-API.
type clientImpl struct {
	runtimeClient v1.RuntimeServiceClient
	imageClient   v1.ImageServiceClient
	conn          *grpc.ClientConn
	initRetry     retry.Retrier
	socketPath    string
}

// NewCRIOClient creates a new CRI-O client implementing the CRIOItf interface.
func NewCRIOClient() (Client, error) {
	socketPath := getCRIOSocketPath()
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
		return nil, fmt.Errorf("failed to initialize CRI-O client on socket %s: %w", socketPath, err)
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
	return containersResponse.Containers, nil
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
func (c *clientImpl) GetContainerImage(ctx context.Context, imageSpec *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error) {
	imageStatusResponse, err := c.imageClient.ImageStatus(ctx, &v1.ImageStatusRequest{Image: imageSpec, Verbose: verbose})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image status for spec %s: %w", imageSpec.Image, err)
	}
	if imageStatusResponse.Image == nil {
		return nil, fmt.Errorf("image not found for spec %s", imageSpec.Image)
	}
	return imageStatusResponse, nil
}

// GetPodStatus retrieves the status of a specific pod sandbox.
func (c *clientImpl) GetPodStatus(ctx context.Context, podSandboxID string) (*v1.PodSandboxStatus, error) {
	podSandboxStatusResponse, err := c.runtimeClient.PodSandboxStatus(ctx, &v1.PodSandboxStatusRequest{PodSandboxId: podSandboxID})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod status for pod ID %s: %w", podSandboxID, err)
	}
	return podSandboxStatusResponse.Status, nil
}

// GetCRIOImageLayers returns the paths of each layer's `diff` directory in the correct order.
func (c *clientImpl) GetCRIOImageLayers(imgMeta *workloadmeta.ContainerImageMetadata) ([]string, error) {
	var lowerDirs []string

	digestToIDMap, err := c.buildDigestToIDMap(imgMeta)
	if err != nil {
		return nil, fmt.Errorf("failed to build digest to ID map: %w", err)
	}

	// Construct the list of lowerDirs by mapping each layer to its corresponding `diff` directory path
	for _, layer := range imgMeta.Layers {
		layerID, found := digestToIDMap[layer.Digest]
		if !found {
			return nil, fmt.Errorf("layer ID not found for digest %s", layer.Digest)
		}

		layerPath := filepath.Join(overlayPath, layerID, "diff")
		lowerDirs = append([]string{layerPath}, lowerDirs...)
	}

	return lowerDirs, nil
}

// getCRIOSocketPath returns the configured CRI-O socket path or the default path.
func getCRIOSocketPath() string {
	criSocket := pkgconfigsetup.Datadog().GetString("cri_socket_path")
	if criSocket == "" {
		return defaultCrioSocketPath
	}
	return criSocket
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

// buildDigestToIDMap creates a map of layer digests to IDs for the layers in imgMeta.
func (c *clientImpl) buildDigestToIDMap(imgMeta *workloadmeta.ContainerImageMetadata) (map[string]string, error) {
	file, err := os.Open(overlayLayersPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open layers.json: %w", err)
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read layers.json: %w", err)
	}

	var layers []layerInfo
	if err := json.Unmarshal(fileBytes, &layers); err != nil {
		return nil, fmt.Errorf("failed to parse layers.json: %w", err)
	}

	neededDigests := make(map[string]struct{})
	for _, layer := range imgMeta.Layers {
		neededDigests[layer.Digest] = struct{}{}
	}

	digestToIDMap := make(map[string]string)
	for _, layer := range layers {
		if _, found := neededDigests[layer.DiffDigest]; found {
			digestToIDMap[layer.DiffDigest] = layer.ID
		}
	}

	return digestToIDMap, nil
}

// layerInfo represents each entry in layers.json
type layerInfo struct {
	ID         string `json:"id"`
	DiffDigest string `json:"diff-digest"`
}

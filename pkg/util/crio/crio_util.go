// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio

// Package crio provides a crio client.
package crio

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	containersimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	defaultCrioSocketPath = "/var/run/crio/crio.sock"
	udsPrefix             = "unix://%s"
	overlayPath           = "/var/lib/containers/storage/overlay"
	overlayImagePath      = "/var/lib/containers/storage/overlay-images"
	overlayLayersPath     = "/var/lib/containers/storage/overlay-layers/layers.json"
)

// Client defines an interface for interacting with the CRI-API, providing methods for
// retrieving information about container and pod statuses, images, and metadata.
type Client interface {
	// RuntimeMetadata returns metadata about the container runtime, including version details.
	// Accepts a context to manage request lifetime.
	RuntimeMetadata(ctx context.Context) (*v1.VersionResponse, error)

	// GetAllContainers lists all containers managed by CRI-O.
	// Accepts a context for managing request lifetime and returns a slice of container metadata.
	GetAllContainers(ctx context.Context) ([]*v1.Container, error)

	// GetContainerStatus retrieves the status of a specified container by containerID.
	// Accepts a context for the request and returns details on container state, creation time, and exit codes.
	GetContainerStatus(ctx context.Context, containerID string) (*v1.ContainerStatusResponse, error)

	// GetContainerImage fetches metadata for a specified image, identified by imageSpec.
	// Accepts a context, the imageSpec to identify the image, and a verbose flag for detailed metadata.
	GetContainerImage(ctx context.Context, imageSpec *v1.ImageSpec, verbose bool) (*v1.ImageStatusResponse, error)

	// GetPodStatus provides the status of a specified pod sandbox, identified by podSandboxID.
	// Takes a context to manage the request and returns sandbox status information.
	GetPodStatus(ctx context.Context, podSandboxID string) (*v1.PodSandboxStatus, error)

	// GetCRIOImageLayers returns paths to `diff` directories for each layer of the specified image,
	// using imgMeta to identify the image and resolve its layers.
	GetCRIOImageLayers(imgMeta *workloadmeta.ContainerImageMetadata) ([]string, error)

	// ListImages retrieves all images available in the CRI-O runtime.
	// Accepts a context for request management and returns a slice of image metadata.
	ListImages(ctx context.Context) ([]*v1.Image, error)
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
		if layer.Digest == "" { // Skip empty layers
			continue
		}
		layerID, found := digestToIDMap[layer.Digest]
		if !found {
			return nil, fmt.Errorf("layer ID not found for digest %s", layer.Digest)
		}

		layerPath := filepath.Join(GetOverlayPath(), layerID, "diff")
		lowerDirs = append([]string{layerPath}, lowerDirs...)
	}

	return lowerDirs, nil
}

// GetOverlayImagePath returns the path to the overlay-images directory.
func GetOverlayImagePath() string {
	if env.IsContainerized() {
		return containersimage.SanitizeHostPath(overlayImagePath)
	}
	return overlayImagePath
}

// GetOverlayPath returns the path to the overlay directory.
func GetOverlayPath() string {
	if env.IsContainerized() {
		return containersimage.SanitizeHostPath(overlayPath)
	}
	return overlayPath
}

// GetOverlayLayersPath returns the path to the overlay-layers directory.
func GetOverlayLayersPath() string {
	if env.IsContainerized() {
		return containersimage.SanitizeHostPath(overlayLayersPath)
	}
	return overlayLayersPath
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
		return errors.New("connection not in READY state")
	}

	return nil
}

// buildDigestToIDMap creates a map of layer digests to IDs for the layers in imgMeta.
func (c *clientImpl) buildDigestToIDMap(imgMeta *workloadmeta.ContainerImageMetadata) (map[string]string, error) {
	file, err := os.Open(GetOverlayLayersPath())
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
		if layer.Digest != "" { // Skip empty layers
			neededDigests[layer.Digest] = struct{}{}
		}
	}

	digestToIDMap := make(map[string]string)
	for _, layer := range layers {
		if _, found := neededDigests[layer.DiffDigest]; found {
			digestToIDMap[layer.DiffDigest] = layer.ID
		}
	}

	return digestToIDMap, nil
}

// ListImages retrieves all images available in the CRI-O runtime.
func (c *clientImpl) ListImages(ctx context.Context) ([]*v1.Image, error) {
	imageListResponse, err := c.imageClient.ListImages(ctx, &v1.ListImagesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}
	return imageListResponse.GetImages(), nil
}

// layerInfo represents each entry in layers.json
type layerInfo struct {
	ID         string `json:"id"`
	DiffDigest string `json:"diff-digest"`
}

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
	"runtime"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	podresourcesv1 "k8s.io/kubelet/pkg/apis/podresources/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

// PodResourcesClient is a small wrapper for the PodResources kubernetes API
type PodResourcesClient struct {
	conn   *grpc.ClientConn
	client podresourcesv1.PodResourcesListerClient
}

// ContainerKey is a struct that represents a unique container
type ContainerKey struct {
	// Namespace is the namespace of the pod
	Namespace string

	// PodName is the name of the pod
	PodName string

	// ContainerName is the name of the container
	ContainerName string
}

// NewPodResourcesClient creates a new PodResourcesClient using the socket path
// from the configuration. Will fail if the socket path is not set.
func NewPodResourcesClient(config config.Component) (*PodResourcesClient, error) {
	podResourcesSocket := config.GetString("kubernetes_kubelet_podresources_socket")
	if podResourcesSocket == "" {
		return nil, errors.New("kubernetes_kubelet_podresources_socket is not set")
	}

	socketPrefix := "unix://"
	if runtime.GOOS == "windows" {
		socketPrefix = "npipe://"
	}
	return NewPodResourcesClientWithSocket(socketPrefix + podResourcesSocket)
}

// NewPodResourcesClientWithSocket creates a new PodResourcesClient using the
// provided socket path (must start with unix:// on Linux or npipe:// on Windows).
func NewPodResourcesClientWithSocket(socket string) (*PodResourcesClient, error) {
	conn, err := grpc.NewClient(
		socket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)),
	)
	if err != nil {
		return nil, fmt.Errorf("failure creating gRPC client to '%s': %w", socket, err)
	}

	client := podresourcesv1.NewPodResourcesListerClient(conn)

	return &PodResourcesClient{
		conn:   conn,
		client: client,
	}, nil
}

// Close closes the connection to the gRPC server.
func (c *PodResourcesClient) Close() {
	c.conn.Close()
}

// ListPodResources returns a list of PodResources from the gRPC server.
func (c *PodResourcesClient) ListPodResources(ctx context.Context) ([]*podresourcesv1.PodResources, error) {
	resp, err := c.client.List(ctx, &podresourcesv1.ListPodResourcesRequest{})
	return resp.GetPodResources(), err
}

// GetContainerResourcesMap returns the PodResources allocations assigned to each container.
func (c *PodResourcesClient) GetContainerResourcesMap(ctx context.Context) (map[ContainerKey][]ContainerAllocatedResource, error) {
	pods, err := c.ListPodResources(ctx)
	if err != nil {
		return nil, err
	}

	containerResourceMap := make(map[ContainerKey][]ContainerAllocatedResource)
	for _, pod := range pods {
		for _, container := range pod.GetContainers() {
			resources := extractAllocatedResources(container)
			if len(resources) == 0 {
				continue
			}

			key := ContainerKey{
				Namespace:     pod.GetNamespace(),
				PodName:       pod.GetName(),
				ContainerName: container.GetName(),
			}
			containerResourceMap[key] = resources
		}
	}

	return containerResourceMap, nil
}

func extractAllocatedResources(container *podresourcesv1.ContainerResources) []ContainerAllocatedResource {
	resources := make([]ContainerAllocatedResource, 0)
	for _, device := range container.GetDevices() {
		for _, id := range device.GetDeviceIds() {
			resources = append(resources, ContainerAllocatedResource{
				Name: device.GetResourceName(),
				ID:   id,
			})
		}
	}
	for _, dynamicResource := range container.GetDynamicResources() {
		for _, claimResource := range dynamicResource.GetClaimResources() {
			resources = append(resources, ContainerAllocatedResource{
				Name: claimResource.GetDriverName(),
				ID:   claimResource.GetDeviceName(),
			})
		}
	}
	return resources
}

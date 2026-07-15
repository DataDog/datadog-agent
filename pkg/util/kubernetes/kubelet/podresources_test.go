// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	podresourcesv1 "k8s.io/kubelet/pkg/apis/podresources/v1"
)

type fakePodResourcesListerClient struct {
	podResources []*podresourcesv1.PodResources
	err          error
}

func (f fakePodResourcesListerClient) List(_ context.Context, _ *podresourcesv1.ListPodResourcesRequest, _ ...grpc.CallOption) (*podresourcesv1.ListPodResourcesResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &podresourcesv1.ListPodResourcesResponse{PodResources: f.podResources}, nil
}

func (f fakePodResourcesListerClient) GetAllocatableResources(context.Context, *podresourcesv1.AllocatableResourcesRequest, ...grpc.CallOption) (*podresourcesv1.AllocatableResourcesResponse, error) {
	return nil, errors.New("not implemented")
}

func (f fakePodResourcesListerClient) Get(context.Context, *podresourcesv1.GetPodResourcesRequest, ...grpc.CallOption) (*podresourcesv1.GetPodResourcesResponse, error) {
	return nil, errors.New("not implemented")
}

func TestGetContainerResourcesMap(t *testing.T) {
	ctx := context.Background()
	client := &PodResourcesClient{
		client: fakePodResourcesListerClient{podResources: []*podresourcesv1.PodResources{
			{
				Name:      "pod",
				Namespace: "namespace",
				Containers: []*podresourcesv1.ContainerResources{
					{
						Name: "legacy",
						Devices: []*podresourcesv1.ContainerDevices{
							{
								ResourceName: "nvidia.com/gpu",
								DeviceIds:    []string{"GPU-1"},
							},
						},
					},
					{
						Name: "dra",
						DynamicResources: []*podresourcesv1.DynamicResource{
							{
								ClaimName:      "claim",
								ClaimNamespace: "namespace",
								ClaimResources: []*podresourcesv1.ClaimResource{
									{
										DriverName: "gpu.nvidia.com",
										DeviceName: "gpu-0",
									},
								},
							},
						},
					},
					{
						Name: "mixed",
						Devices: []*podresourcesv1.ContainerDevices{
							{
								ResourceName: "example.com/device",
								DeviceIds:    []string{"device-1"},
							},
						},
						DynamicResources: []*podresourcesv1.DynamicResource{
							{
								ClaimName:      "claim-2",
								ClaimNamespace: "namespace",
								ClaimResources: []*podresourcesv1.ClaimResource{
									{
										DriverName: "gpu.nvidia.com",
										DeviceName: "gpu-1",
									},
								},
							},
						},
					},
					{Name: "empty"},
				},
			},
		}},
	}

	resources, err := client.GetContainerResourcesMap(ctx)
	require.NoError(t, err)
	require.Len(t, resources, 3)

	legacy := resources[ContainerKey{Namespace: "namespace", PodName: "pod", ContainerName: "legacy"}]
	require.Equal(t, []ContainerAllocatedResource{
		{Name: "nvidia.com/gpu", ID: "GPU-1"},
	}, legacy)

	dra := resources[ContainerKey{Namespace: "namespace", PodName: "pod", ContainerName: "dra"}]
	require.Equal(t, []ContainerAllocatedResource{
		{Name: "gpu.nvidia.com", ID: "gpu-0"},
	}, dra)

	mixed := resources[ContainerKey{Namespace: "namespace", PodName: "pod", ContainerName: "mixed"}]
	require.Equal(t, []ContainerAllocatedResource{
		{Name: "example.com/device", ID: "device-1"},
		{Name: "gpu.nvidia.com", ID: "gpu-1"},
	}, mixed)
}

func TestAddResourcesToContainerList(t *testing.T) {
	ku := &KubeUtil{}
	pod := &Pod{
		Metadata: PodMetadata{
			Namespace: "namespace",
			Name:      "pod",
		},
	}
	containers := []ContainerStatus{{Name: "container"}}

	ku.addResourcesToContainerList(map[ContainerKey][]ContainerAllocatedResource{
		{Namespace: "namespace", PodName: "pod", ContainerName: "container"}: {
			{Name: "nvidia.com/gpu", ID: "GPU-1"},
			{Name: "gpu.nvidia.com", ID: "gpu-0"},
		},
	}, pod, containers)

	require.Equal(t, []ContainerAllocatedResource{
		{Name: "nvidia.com/gpu", ID: "GPU-1"},
		{Name: "gpu.nvidia.com", ID: "gpu-0"},
	}, containers[0].ResolvedAllocatedResources)
}

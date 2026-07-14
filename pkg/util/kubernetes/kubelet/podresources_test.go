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
										PoolName:   "node-a",
										DeviceName: "gpu-0",
										ShareId:    ptr("share-1"),
										CdiDevices: []*podresourcesv1.CDIDevice{
											{Name: "gpu.nvidia.com/device=claim_gpu-0"},
										},
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
										PoolName:   "node-a",
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
	require.Len(t, legacy.Devices, 1)
	require.Equal(t, "nvidia.com/gpu", legacy.Devices[0].GetResourceName())
	require.Empty(t, legacy.DynamicResources)

	dra := resources[ContainerKey{Namespace: "namespace", PodName: "pod", ContainerName: "dra"}]
	require.Empty(t, dra.Devices)
	require.Len(t, dra.DynamicResources, 1)
	require.Equal(t, ContainerDynamicResource{
		ClaimName:      "claim",
		ClaimNamespace: "namespace",
		ClaimResources: []ContainerClaimResource{
			{
				DriverName: "gpu.nvidia.com",
				PoolName:   "node-a",
				DeviceName: "gpu-0",
				ShareID:    "share-1",
				CDIDevices: []string{"gpu.nvidia.com/device=claim_gpu-0"},
			},
		},
	}, dra.DynamicResources[0])

	mixed := resources[ContainerKey{Namespace: "namespace", PodName: "pod", ContainerName: "mixed"}]
	require.Len(t, mixed.Devices, 1)
	require.Len(t, mixed.DynamicResources, 1)
}

func ptr[T any](value T) *T {
	return &value
}

type fakeDRAResourceResolver struct {
	input    ContainerClaimResource
	resolved ContainerAllocatedResource
}

func (f fakeDRAResourceResolver) ResolveDRAResource(resource ContainerClaimResource) (ContainerAllocatedResource, bool) {
	if resource.DriverName != f.input.DriverName || resource.PoolName != f.input.PoolName || resource.DeviceName != f.input.DeviceName {
		return ContainerAllocatedResource{}, false
	}
	return f.resolved, true
}

func TestAddResourcesToContainerListResolvesDRA(t *testing.T) {
	resolverInput := ContainerClaimResource{
		DriverName: "gpu.nvidia.com",
		PoolName:   "node-a",
		DeviceName: "gpu-0",
	}
	resolvedResource := ContainerAllocatedResource{Name: "gpu.nvidia.com", ID: "GPU-1"}
	ku := &KubeUtil{
		draResourceResolver: fakeDRAResourceResolver{
			input:    resolverInput,
			resolved: resolvedResource,
		},
	}
	pod := &Pod{
		Metadata: PodMetadata{
			Namespace: "namespace",
			Name:      "pod",
		},
	}
	containers := []ContainerStatus{{Name: "container"}}

	ku.addResourcesToContainerList(map[ContainerKey]ContainerPodResources{
		{Namespace: "namespace", PodName: "pod", ContainerName: "container"}: {
			Devices: []*podresourcesv1.ContainerDevices{
				{
					ResourceName: "nvidia.com/gpu",
					DeviceIds:    []string{"GPU-1"},
				},
			},
			DynamicResources: []ContainerDynamicResource{
				{
					ClaimName:      "claim",
					ClaimNamespace: "namespace",
					ClaimResources: []ContainerClaimResource{resolverInput},
				},
			},
		},
	}, pod, containers)

	require.Equal(t, []ContainerAllocatedResource{{Name: "nvidia.com/gpu", ID: "GPU-1"}, resolvedResource}, containers[0].ResolvedAllocatedResources)
	require.Len(t, containers[0].DynamicAllocatedResources, 1)
}

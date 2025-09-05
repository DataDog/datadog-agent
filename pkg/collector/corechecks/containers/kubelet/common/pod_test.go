// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package common

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

var staticKubeletConfig = workloadmeta.KubeletConfigDocument{
	KubeletConfig: workloadmeta.KubeletConfigSpec{
		CPUManagerPolicy: "static",
	},
}

var noneKubeletConfig = workloadmeta.KubeletConfigDocument{
	KubeletConfig: workloadmeta.KubeletConfigSpec{
		CPUManagerPolicy: "none",
	},
}

var requestedWholeCores = workloadmeta.ContainerResources{
	RequestedWholeCores: pointer.Ptr(true),
}

var requestedPartialCores = workloadmeta.ContainerResources{
	RequestedWholeCores: pointer.Ptr(false),
}

var nilRequestedWholeCores = workloadmeta.ContainerResources{
	RequestedWholeCores: nil,
}

func TestAppendKubeRequestedCPUManagementTag(t *testing.T) {
	tests := []struct {
		name         string
		qos          string
		containerID  types.EntityID
		initialTags  []string
		setupMock    func(mockStore workloadmetamock.Mock)
		expectedTags []string
	}{
		{
			name:        "nil kubelet entity returns original tags",
			qos:         "Guaranteed",
			containerID: types.NewEntityID(types.ContainerID, "container-123"),
			initialTags: []string{"existing:tag"},
			setupMock: func(_ workloadmetamock.Mock) {
				// Don't set any kubelet, so GetKubelet will return nil
			},
			expectedTags: []string{"existing:tag"},
		},
		{
			name:        "nil container returns original tags",
			qos:         "Guaranteed",
			containerID: types.NewEntityID(types.ContainerID, "container-123"),
			initialTags: []string{"existing:tag"},
			setupMock: func(mockStore workloadmetamock.Mock) {
				// Set up kubelet with static policy
				mockStore.Set(&workloadmeta.Kubelet{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubelet,
						ID:   workloadmeta.KubeletID,
					},
					ConfigDocument: staticKubeletConfig,
				})
				// Don't set up container, so GetContainer will return nil
			},
			expectedTags: []string{"existing:tag"},
		},
		{
			name:        "guaranteed qos with whole cores and static policy adds kube_static_cpus:true tag",
			qos:         "Guaranteed",
			containerID: types.NewEntityID(types.ContainerID, "container-123"),
			initialTags: []string{"existing:tag"},
			setupMock: func(mockStore workloadmetamock.Mock) {
				// Set up kubelet with static policy
				mockStore.Set(&workloadmeta.Kubelet{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubelet,
						ID:   workloadmeta.KubeletID,
					},
					ConfigDocument: staticKubeletConfig,
				})
				// Set up container with whole cores requested
				mockStore.Set(&workloadmeta.Container{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "container-123",
					},
					Resources: requestedWholeCores,
				})
			},
			expectedTags: []string{"existing:tag", "kube_static_cpus:true"},
		},
		{
			name:        "guaranteed qos with whole cores but non-static policy adds kube_static_cpus:false tag",
			qos:         "Guaranteed",
			containerID: types.NewEntityID(types.ContainerID, "container-123"),
			initialTags: []string{"existing:tag"},
			setupMock: func(mockStore workloadmetamock.Mock) {
				// Set up kubelet with none policy
				mockStore.Set(&workloadmeta.Kubelet{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubelet,
						ID:   workloadmeta.KubeletID,
					},
					ConfigDocument: noneKubeletConfig,
				})
				// Set up container with whole cores requested
				mockStore.Set(&workloadmeta.Container{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "container-123",
					},
					Resources: requestedWholeCores,
				})
			},
			expectedTags: []string{"existing:tag", "kube_static_cpus:false"},
		},
		{
			name:        "guaranteed qos with static policy but partial cores adds kube_static_cpus:false tag",
			qos:         "Guaranteed",
			containerID: types.NewEntityID(types.ContainerID, "container-123"),
			initialTags: []string{"existing:tag"},
			setupMock: func(mockStore workloadmetamock.Mock) {
				// Set up kubelet with static policy
				mockStore.Set(&workloadmeta.Kubelet{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubelet,
						ID:   workloadmeta.KubeletID,
					},
					ConfigDocument: staticKubeletConfig,
				})
				// Set up container with partial cores requested
				mockStore.Set(&workloadmeta.Container{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "container-123",
					},
					Resources: requestedPartialCores,
				})
			},
			expectedTags: []string{"existing:tag", "kube_static_cpus:false"},
		},
		{
			name:        "guaranteed qos with static policy but nil whole cores adds kube_static_cpus:false tag",
			qos:         "Guaranteed",
			containerID: types.NewEntityID(types.ContainerID, "container-123"),
			initialTags: []string{"existing:tag"},
			setupMock: func(mockStore workloadmetamock.Mock) {
				// Set up kubelet with static policy
				mockStore.Set(&workloadmeta.Kubelet{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubelet,
						ID:   workloadmeta.KubeletID,
					},
					ConfigDocument: staticKubeletConfig,
				},
				)
				// Set up container with nil whole cores requested
				mockStore.Set(&workloadmeta.Container{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "container-123",
					},
					Resources: nilRequestedWholeCores,
				})
			},
			expectedTags: []string{"existing:tag", "kube_static_cpus:false"},
		},
		{
			name:        "non-guaranteed qos with whole cores and static policy adds kube_static_cpus:false tag",
			qos:         "Burstable",
			containerID: types.NewEntityID(types.ContainerID, "container-123"),
			initialTags: []string{"existing:tag"},
			setupMock: func(mockStore workloadmetamock.Mock) {
				// Set up kubelet with static policy
				mockStore.Set(&workloadmeta.Kubelet{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubelet,
						ID:   workloadmeta.KubeletID,
					},
					ConfigDocument: staticKubeletConfig,
				},
				)
				// Set up container with whole cores requested
				mockStore.Set(&workloadmeta.Container{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindContainer,
						ID:   "container-123",
					},
					Resources: requestedWholeCores,
				})
			},
			expectedTags: []string{"existing:tag", "kube_static_cpus:false"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				core.MockBundle(),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			tt.setupMock(mockStore)
			result := AppendKubeStaticCPUsTag(mockStore, tt.qos, tt.containerID, tt.initialTags)
			assert.Equal(t, tt.expectedTags, result, fmt.Sprintf("expected %v, got %v", tt.expectedTags, result))
		})
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package k8s

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	model "github.com/DataDog/agent-payload/v5/process"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

func TestNodeCollector(t *testing.T) {
	timestamp := metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC)) // 1389744000

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			UID:               types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
			Name:              "node",
			CreationTimestamp: timestamp,
			Labels: map[string]string{
				"app": "my-app",
			},
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			ResourceVersion: "1216",
		},
		Spec: corev1.NodeSpec{
			PodCIDR:       "1234-5678-90",
			Unschedulable: true,
			Taints: []corev1.Taint{{
				Key:    "taint2NoTimeStamp",
				Value:  "val1",
				Effect: "effect1",
			}},
		},
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{
				KernelVersion:           "kernel1",
				OSImage:                 "os1",
				ContainerRuntimeVersion: "docker1",
				KubeletVersion:          "1.18",
				OperatingSystem:         "linux",
				Architecture:            "amd64",
			},
			Addresses: []corev1.NodeAddress{{
				Type:    "endpoint",
				Address: "1234567890",
			}},
			Images: []corev1.ContainerImage{{
				Names:     []string{"image1"},
				SizeBytes: 10,
			}},
			DaemonEndpoints: corev1.NodeDaemonEndpoints{KubeletEndpoint: corev1.DaemonEndpoint{Port: 11}},
			Capacity: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourcePods:   resource.MustParse("100"),
				corev1.ResourceCPU:    resource.MustParse("10"),
				corev1.ResourceMemory: resource.MustParse("10Gi"),
			},
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourcePods:   resource.MustParse("50"),
				corev1.ResourceCPU:    resource.MustParse("5"),
				corev1.ResourceMemory: resource.MustParse("5G"),
			},
			Conditions: []corev1.NodeCondition{{
				Type:               corev1.NodeReady,
				Status:             corev1.ConditionTrue,
				LastHeartbeatTime:  timestamp,
				LastTransitionTime: timestamp,
				Reason:             "node to ready",
				Message:            "ready",
			}},
		},
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewNodeCollector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{node},
		ExpectedMetadataType:       &model.CollectorNode{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && !orchestrator

package kubelet

import (
	"context"
	"io"

	kubeletv1alpha1 "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
)

// KubeUtilInterface defines the interface for kubelet api
// see `kubelet_orchestrator` for orchestrator-only interface
type KubeUtilInterface interface {
	GetNodename(ctx context.Context) (string, error)
	GetLocalPodList(ctx context.Context) ([]*Pod, error)
	GetLocalPodListWithMetadata(ctx context.Context) (*PodList, error)
	QueryKubelet(ctx context.Context, path string) ([]byte, int, error)
	GetRawConnectionInfo() map[string]string
	GetRawMetrics(ctx context.Context) ([]byte, error)
	GetLocalStatsSummary(ctx context.Context) (*kubeletv1alpha1.Summary, error)
	StreamLogs(ctx context.Context, podNamespace, podName, containerName string, logOptions *StreamLogOptions) (io.ReadCloser, error)
	GetConfig(ctx context.Context) ([]byte, *ConfigDocument, error)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator && kubeapiserver && kubelet

package pod

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// listLocalPods returns the pods of the node the Agent runs on.
func listLocalPods(ctx context.Context) ([]*corev1.Pod, error) {
	kubeUtil, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}
	return kubeUtil.GetRawLocalPodList(ctx)
}

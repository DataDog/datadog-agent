// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubelet

package agentlifecycleimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

type kubeletLocalPodSource struct{}

func newLocalPodSource() localPodSource {
	return kubeletLocalPodSource{}
}

func (kubeletLocalPodSource) ListLocalPods(ctx context.Context) ([]localPod, error) {
	kubeUtil, err := kubelet.GetKubeUtil()
	if err != nil {
		return nil, err
	}
	kubeletPods, err := kubeUtil.GetLocalPodList(ctx)
	if err != nil {
		return nil, err
	}

	pods := make([]localPod, 0, len(kubeletPods))
	for _, pod := range kubeletPods {
		if pod == nil {
			continue
		}
		owners := make([]podOwner, 0, len(pod.Owners()))
		for _, owner := range pod.Owners() {
			owners = append(owners, podOwner{
				kind:       owner.Kind,
				uid:        owner.ID,
				controller: owner.Controller != nil && *owner.Controller,
			})
		}
		pods = append(pods, localPod{
			uid:       pod.Metadata.UID,
			name:      pod.Metadata.Name,
			namespace: pod.Metadata.Namespace,
			createdAt: pod.Metadata.CreationTimestamp,
			owners:    owners,
		})
	}
	return pods, nil
}

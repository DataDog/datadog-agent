// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubelet && orchestrator

package flare

import (
	"context"
	"encoding/json"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/redact"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const kubeletTimeout = 30 * time.Second

func getKubeletConfig() (data []byte, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), kubeletTimeout)
	defer cancel()

	ku, err := kubelet.GetKubeUtil()
	if err != nil {
		// If we can’t reach the kubelet, let’s do nothing
		log.Debugf("Could not get kubelet client: %v", err)
		return nil, nil
	}
	data, _, err = ku.QueryKubelet(ctx, "/configz")
	return
}

func getKubeletPods() (data []byte, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), kubeletTimeout)
	defer cancel()

	ku, err := kubelet.GetKubeUtil()
	if err != nil {
		// If we can’t reach the kubelet, let’s do nothing
		log.Debugf("Could not get kubelet client: %v", err)
		return nil, nil
	}

	// Get the raw local pod list from the kubelet
	pods, err := ku.GetRawLocalPodList(ctx)
	if err != nil {
		log.Debugf("Could not get raw local pod list: %v", err)
		return nil, err
	}

	scrubber := redact.NewDefaultDataScrubber()
	for _, pod := range pods {
		redact.ScrubPod(pod, scrubber)
	}

	// Create a new pod list with the scrubbed pods
	podList := &v1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PodList",
			APIVersion: "v1",
		},
		Items: make([]v1.Pod, len(pods)),
	}
	for i, pod := range pods {
		podList.Items[i] = *pod
	}

	data, err = json.Marshal(podList)
	if err != nil {
		log.Debugf("Could not marshal pod list: %v", err)
		return nil, err
	}
	return
}

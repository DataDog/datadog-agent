// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubelet

package flare

import (
	"context"
	"time"

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
	data, _, err = ku.QueryKubelet(ctx, "/pods")
	return
}

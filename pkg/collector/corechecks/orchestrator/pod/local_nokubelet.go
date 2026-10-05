// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator && kubeapiserver && !kubelet

package pod

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
)

// listLocalPods returns an error: the pods of the local node are listed from the kubelet.
func listLocalPods(context.Context) ([]*corev1.Pod, error) {
	return nil, errors.New("collecting the pods of the local node requires the kubelet, set node_selector to collect the pods of other nodes")
}

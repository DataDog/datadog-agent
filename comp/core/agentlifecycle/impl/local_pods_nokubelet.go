// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !kubelet

package agentlifecycleimpl

import (
	"context"
	"errors"
)

type unsupportedLocalPodSource struct{}

func newLocalPodSource() localPodSource {
	return unsupportedLocalPodSource{}
}

func (unsupportedLocalPodSource) ListLocalPods(context.Context) ([]localPod, error) {
	return nil, errors.New("experimental node Agent rollout requires an Agent built with kubelet support")
}

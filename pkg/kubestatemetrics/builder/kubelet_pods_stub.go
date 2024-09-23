// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && !kubelet

package builder

import (
	"context"

	"k8s.io/client-go/tools/cache"
)

// When the Kubelet flag is not set, we don't need a kubeletReflector, so we can
// return a struct that does nothing

type kubeletReflector struct{}

func newKubeletReflector(_ []string) (kubeletReflector, error) {
	return kubeletReflector{}, nil
}

func (kr *kubeletReflector) addStore(_ cache.Store) error {
	return nil
}

func (kr *kubeletReflector) start(_ context.Context) error {
	return nil
}

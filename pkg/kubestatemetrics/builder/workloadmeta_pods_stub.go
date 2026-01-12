// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && !kubelet

package builder

import (
	"context"

	"k8s.io/client-go/tools/cache"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// When the Kubelet flag is not set, we don't need a workloadmetaReflector, so
// we can return a struct that does nothing

type workloadmetaReflector struct{}

func newWorkloadmetaReflector(wmeta workloadmeta.Component, namespaces []string) (workloadmetaReflector, error) {
	return workloadmetaReflector{}, nil
}

func (wr *workloadmetaReflector) addStore(_ cache.Store) error {
	return nil
}

func (wr *workloadmetaReflector) start(_ context.Context) error {
	return nil
}

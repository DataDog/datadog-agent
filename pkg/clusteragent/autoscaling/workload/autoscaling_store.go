// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

var (
	// AutoscalingStore is the store for Datadog Pod Autoscalers
	AutoscalingStore *autoscaling.Store[model.PodAutoscalerInternal]
	// AutoscalingStoreOnce is used to init the store once
	AutoscalingStoreOnce sync.Once
)

type autoscalingStore = autoscaling.Store[model.PodAutoscalerInternal]

// GetAutoscalingStore returns the autoscaling store, init once
func GetAutoscalingStore(ctx context.Context) *autoscaling.Store[model.PodAutoscalerInternal] {
	AutoscalingStoreOnce.Do(func() {
		AutoscalingStore = autoscaling.NewStore[model.PodAutoscalerInternal]()
	})
	return AutoscalingStore
}

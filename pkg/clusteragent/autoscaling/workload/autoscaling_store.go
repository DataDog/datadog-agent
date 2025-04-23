// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

var (
	// AutoscalingStore is the store for Datadog Pod Autoscalers
	AutoscalingStore *store
	// AutoscalingStoreOnce is used to init the store once
	AutoscalingStoreOnce sync.Once
)

// GetAutoscalingStore returns the autoscaling store, init once
func GetAutoscalingStore() *store {
	AutoscalingStoreOnce.Do(func() {
		AutoscalingStore = autoscaling.NewStore[model.PodAutoscalerInternal]()
	})
	return AutoscalingStore
}

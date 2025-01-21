// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package local

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/loadstore"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	pollingInterval           = 180 * time.Second
	localRecommenderID string = "lr"
)

type recommender struct {
	replicaCalculator replicaCalculator
	store             *autoscaling.Store[model.PodAutoscalerInternal]
	context           context.Context
}

// NewInterface creates a new RecommenderInterface
func NewInterface(podWatcher common.PodWatcher, store *autoscaling.Store[model.PodAutoscalerInternal]) *recommender {
	replicaCalculator := newReplicaCalculator(podWatcher)

	return &recommender{
		replicaCalculator: replicaCalculator,
		store:             store,
	}
}

// Run starts the recommender interface to generate local recommendations
func (r *recommender) Run(ctx context.Context) {
	if ctx == nil {
		log.Errorf("Cannot run with a nil context")
		return
	}
	r.context = ctx

	log.Infof("Starting autoscaling interface")
	ticker := time.NewTicker(pollingInterval)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				r.process(ctx)
			case <-ctx.Done():
				log.Infof("Stopping autoscaling interface")
				return
			}
		}
	}()
}

func (r *recommender) process(ctx context.Context) {
	// Call loadstore when processing
	lStore := loadstore.GetWorkloadMetricStore(ctx)
	if lStore == nil {
		log.Debugf("Local metrics store not initialized, skipping calculations.")
		return
	}

	podAutoscalers := r.store.GetAll()

	for _, podAutoscaler := range podAutoscalers {
		if podAutoscaler.CustomRecommenderConfiguration() != nil {
			// TODO: Process custom recommender
			continue
		}

		// Lock the store to avoid concurrent writes
		podAutoscalerInternal, _ := r.store.LockRead(podAutoscaler.ID(), true)

		// Generate local recommendations
		horizontalRecommendation, err := r.replicaCalculator.CalculateHorizontalRecommendations(podAutoscalerInternal, lStore)
		if err != nil || horizontalRecommendation == nil {
			log.Debugf("Error calculating horizontal recommendations for pod autoscaler %s: %s", podAutoscalerInternal.ID(), err)
			r.store.Unlock(podAutoscalerInternal.ID())
			continue
		}

		podAutoscalerInternal.UpdateFromLocalValues(*horizontalRecommendation)
		r.store.UnlockSet(podAutoscalerInternal.ID(), podAutoscalerInternal, localRecommenderID)
		log.Debugf("Updated local fallback values for pod autoscaler %s", podAutoscalerInternal.ID())
	}
}

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
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/loadstore"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	pollingInterval           = 30 * time.Second
	localRecommenderID string = "lr"
)

// Recommender is the interface used to generate local recommendations
type Recommender struct {
	replicaCalculator replicaCalculator
	store             *autoscaling.Store[model.PodAutoscalerInternal]
}

// NewRecommender creates a new Recommender to start generating local recommendations
func NewRecommender(podWatcher workload.PodWatcher, store *autoscaling.Store[model.PodAutoscalerInternal]) *Recommender {
	replicaCalculator := newReplicaCalculator(podWatcher)

	return &Recommender{
		replicaCalculator: replicaCalculator,
		store:             store,
	}
}

// Run starts the Recommender interface to generate local recommendations
func (r *Recommender) Run(ctx context.Context) {
	log.Infof("Starting local autoscaling recommender")
	ticker := time.NewTicker(pollingInterval)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				r.process(ctx)
			case <-ctx.Done():
				log.Infof("Stopping local autoscaling recommender")
				return
			}
		}
	}()
}

func (r *Recommender) process(ctx context.Context) {
	// Call loadstore when processing
	lStore := loadstore.GetWorkloadMetricStore(ctx)
	if lStore == nil {
		log.Debugf("Local metrics store not initialized, skipping calculations.")
		return
	}

	localFallbackFilter := func(podAutoscaler model.PodAutoscalerInternal) bool {
		// Only return false if Fallback exists and Horizontal.Enabled is explicitly set to false
		if podAutoscaler.Spec().Fallback != nil && !podAutoscaler.Spec().Fallback.Horizontal.Enabled {
			return false
		}
		return true
	}
	podAutoscalers := r.store.GetFiltered(localFallbackFilter)

	for _, podAutoscaler := range podAutoscalers {
		// Generate local recommendations
		horizontalRecommendation, err := r.replicaCalculator.calculateHorizontalRecommendations(podAutoscaler, lStore)
		r.updateAutoscaler(podAutoscaler.ID(), horizontalRecommendation, err)
		if err != nil {
			log.Debugf("Got error calculating horizontal recommendation for pod autoscaler %s: %v", podAutoscaler.ID(), err)
		} else {
			log.Debugf("Updated local fallback values for pod autoscaler %s", podAutoscaler.ID())
		}
	}
}

func (r *Recommender) updateAutoscaler(key string, horizontalRecommendation *model.HorizontalScalingValues, err error) {
	recommendation := model.ScalingValues{}

	if err != nil {
		recommendation.HorizontalError = err
	}

	if horizontalRecommendation != nil {
		recommendation.Horizontal = horizontalRecommendation
	}

	podAutoscalerInternal, found := r.store.LockRead(key, true)
	if !found { // In case the object is deleted in between when we start calculating
		log.Debugf("Object %s not found in store; local recommendation values not updated", key)
		r.store.Unlock(key)
		return
	}
	podAutoscalerInternal.UpdateFromLocalValues(recommendation)
	r.store.UnlockSet(podAutoscalerInternal.ID(), podAutoscalerInternal, localRecommenderID)
}

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

// Recommender is the interface used to generate local recommendations
type Recommender struct {
	replicaCalculator replicaCalculator
	store             *autoscaling.Store[model.PodAutoscalerInternal]
	context           context.Context
}

// NewRecommender creates a new Recommender to start generating local recommendations
func NewRecommender(podWatcher common.PodWatcher, store *autoscaling.Store[model.PodAutoscalerInternal]) *Recommender {
	replicaCalculator := newReplicaCalculator(podWatcher)

	return &Recommender{
		replicaCalculator: replicaCalculator,
		store:             store,
	}
}

// Run starts the Recommender interface to generate local recommendations
func (r *Recommender) Run(ctx context.Context) {
	if ctx == nil {
		log.Errorf("Cannot run with a nil context")
		return
	}
	r.context = ctx

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

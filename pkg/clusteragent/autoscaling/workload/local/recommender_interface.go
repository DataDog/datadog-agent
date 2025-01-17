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

// RecommenderInterface is the interface for the local recommender
type RecommenderInterface struct {
	localRecommender Recommender
	store            *autoscaling.Store[model.PodAutoscalerInternal]
	context          context.Context
}

// NewInterface creates a new RecommenderInterface
func NewInterface(ctx context.Context, podWatcher common.PodWatcher, store *autoscaling.Store[model.PodAutoscalerInternal]) (*RecommenderInterface, error) {
	localRecommender := newLocalRecommender(podWatcher, loadstore.GetWorkloadMetricStore(ctx))

	return &RecommenderInterface{
		localRecommender: localRecommender,
		store:            store,
	}, nil
}

// Run starts the recommender interface to generate local recommendations
func (ri *RecommenderInterface) Run(ctx context.Context) {
	if ctx == nil {
		log.Errorf("Cannot run with a nil context")
		return
	}
	ri.context = ctx

	log.Infof("Starting autoscaling interface")
	ticker := time.NewTicker(pollingInterval)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ri.process(ctx)
			case <-ctx.Done():
				log.Infof("Stopping autoscaling interface")
				return
			}
		}
	}()
}

func (ri *RecommenderInterface) process(ctx context.Context) {
	podAutoscalers := ri.store.GetAll()
	for _, podAutoscaler := range podAutoscalers {
		if podAutoscaler.CustomRecommenderConfiguration() != nil {
			// TODO: Process custom recommender
			continue
		}
		// Generate local recommendations
		if ri.localRecommender.Store == nil {
			if err := ri.localRecommender.ReinitLoadstore(ctx); err != nil {
				log.Debugf("Skipping local recommendation for pod autoscaler %s: %s", podAutoscaler.ID(), err)
				continue
			}
		}
		horizontalRecommendation, err := ri.localRecommender.CalculateHorizontalRecommendations(podAutoscaler)
		if err != nil || horizontalRecommendation == nil {
			log.Debugf("Error calculating horizontal recommendations for pod autoscaler %s: %s", podAutoscaler.ID(), err)
			continue
		}

		podAutoscaler.UpdateFromLocalValues(*horizontalRecommendation)
		ri.store.Set(podAutoscaler.ID(), podAutoscaler, localRecommenderID)
		log.Debugf("Updated local fallback values for pod autoscaler %s", podAutoscaler.ID())

	}
}

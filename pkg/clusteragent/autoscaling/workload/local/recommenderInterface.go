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
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/loadstore"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/shared"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	pollingInterval           = 180 * time.Second
	localRecommenderID string = "lr"
)

type RecommenderInterface struct {
	localRecommender LocalRecommender
	store            *autoscaling.Store[model.PodAutoscalerInternal]
	context          context.Context
}

func NewInterface(ctx context.Context, podWatcher shared.PodWatcher, store *autoscaling.Store[model.PodAutoscalerInternal]) (*RecommenderInterface, error) {
	localRecommender := LocalRecommender{
		PodWatcher: podWatcher,
		Store:      loadstore.GetWorkloadMetricStore(ctx),
	}

	return &RecommenderInterface{
		localRecommender: localRecommender,
		store:            store,
	}, nil
}

// Run starts the controller to handle objects
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
				log.Debugf("Stopping autoscaling interface")
				return
			}
		}
	}()
	log.Infof("Stopping autoscaling interface")
}

func (ri *RecommenderInterface) process(ctx context.Context) {
	podAutoscalers := ri.store.GetAll()
	for _, podAutoscaler := range podAutoscalers {
		if podAutoscaler.CustomRecommenderConfiguration() != nil {
			// TODO: Process custom recommender
			continue
		} else {
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
			log.Debugf("Updating local fallback values for pod autoscaler %s", podAutoscaler.ID())
			log.Debugf("LOCAL-REC got rec for %s: %+v", podAutoscaler.ID(), horizontalRecommendation)
			podAutoscaler.UpdateFromLocalValues(*horizontalRecommendation)
			ri.store.Set(podAutoscaler.ID(), podAutoscaler, localRecommenderID) // should we be holding the lock here??
			log.Debugf("Updated local fallback values for pod autoscaler %s", podAutoscaler.ID())
		}
	}
}

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
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	pollingInterval           = 180 * time.Second
	localRecommenderID string = "lr"
)

type Interface struct {
	localRecommender localRecommender
	context          context.Context
	store            *autoscaling.Store[model.PodAutoscalerInternal]
}

func newInterface(podWatcher workload.PodWatcher, store *autoscaling.Store[model.PodAutoscalerInternal]) (*Interface, error) {
	localRecommender := localRecommender{
		podWatcher: podWatcher,
	}

	return &Interface{
		localRecommender: localRecommender,
		store:            store,
	}, nil
}

// Run starts the controller to handle objects
func (i *Interface) Run(ctx context.Context, done <-chan struct{}) {
	if ctx == nil {
		log.Errorf("Cannot run with a nil context")
		return
	}
	i.context = ctx

	log.Infof("Starting autoscaling interface")
	ticker := time.NewTicker(pollingInterval)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				i.Process()
			case <-done:
				return
			}
		}
	}()
	log.Infof("Stopping autoscaling interface")
}

func (i *Interface) Process() {
	podAutoscalers := i.store.GetAll()
	for _, podAutoscaler := range podAutoscalers {
		if podAutoscaler.CustomRecommenderConfiguration() != nil {
			// TODO: Process custom recommender
			continue
		} else {
			horizontalRecommendation, err := i.localRecommender.CalculateHorizontalRecommendations(podAutoscaler)
			if err != nil || horizontalRecommendation == nil {
				log.Errorf("Error calculating horizontal recommendations for pod autoscaler %s: %s", podAutoscaler.ID(), err)
				continue
			}
			podAutoscaler.UpdateFromLocalValues(*horizontalRecommendation)
			i.store.UnlockSet(podAutoscaler.ID(), podAutoscaler, localRecommenderID)
		}
	}
}

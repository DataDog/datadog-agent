// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package external

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	pollingInterval              = 30 * time.Second
	externalRecommenderID string = "extr"
)

// Recommender is the interface used to fetch external recommendations
type Recommender struct {
	recommenderClient *recommenderClient
	store             *autoscaling.Store[model.PodAutoscalerInternal]
}

// NewRecommender creates a new Recommender to start fetching external recommendations
func NewRecommender(podWatcher workload.PodWatcher, store *autoscaling.Store[model.PodAutoscalerInternal]) *Recommender {
	recommenderClient := newRecommenderClient(podWatcher)

	return &Recommender{
		recommenderClient: recommenderClient,
		store:             store,
	}
}

// Run starts the Recommender interface to generate local recommendations
func (r *Recommender) Run(ctx context.Context) {
	log.Infof("Starting external autoscaling recommender")
	ticker := time.NewTicker(pollingInterval)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				r.process(ctx)
			case <-ctx.Done():
				log.Infof("Stopping external autoscaling recommender")
				return
			}
		}
	}()
}

func (r *Recommender) process(ctx context.Context) {
	// Filter pod autoscalers to only retrieve autoscalers where external recommender is enabled
	externalRecommendationFilter := func(dpa model.PodAutoscalerInternal) bool {
		return dpa.CustomRecommenderConfiguration() != nil
	}
	podAutoscalers := r.store.GetFiltered(externalRecommendationFilter)

	for _, podAutoscaler := range podAutoscalers {
		// Fetch external recommendations
		recommendation, err := r.recommenderClient.GetReplicaRecommendation(ctx, podAutoscaler)

		r.updateAutoscalerAndUnlock(podAutoscaler.ID(), recommendation, err)
		if err != nil {
			log.Debugf("Got error fetching external recommendation for pod autoscaler %s: %v", podAutoscaler.ID(), err)
		} else {
			log.Debugf("Updated external recommendation for pod autoscaler %s", podAutoscaler.ID())
		}
	}
}

func (r *Recommender) updateAutoscalerAndUnlock(key string, horizontalRecommendation *model.HorizontalScalingValues, err error) {
	recommendation := model.ScalingValues{}
	if err != nil {
		recommendation.HorizontalError = err
	}

	if horizontalRecommendation != nil {
		recommendation.Horizontal = horizontalRecommendation
	}

	podAutoscalerInternal, found := r.store.LockRead(key, true)
	if !found { // In case the object is deleted in between when we start calculating
		log.Debugf("Object %s not found in store; recommendation values not updated", key)
		r.store.Unlock(key)
		return
	}
	podAutoscalerInternal.UpdateFromMainValues(recommendation)
	r.store.UnlockSet(podAutoscalerInternal.ID(), podAutoscalerInternal, externalRecommenderID)
}

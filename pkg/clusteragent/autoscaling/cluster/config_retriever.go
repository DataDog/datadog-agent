// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"context"

	"k8s.io/utils/clock"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	configRetrieverStoreID string = "cacr"
)

// RcClient is a subinterface of rcclient.Component to allow mocking
type RcClient interface {
	SubscribeIgnoreExpiration(product string, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
}

// ConfigRetriever is responsible for retrieving remote objects (Cluster Autoscaling values)
type ConfigRetriever struct {
	isLeader func() bool
	clock    clock.WithTicker

	valuesProcessor autoscalingValuesProcessor
}

// NewConfigRetriever creates a new ConfigRetriever
func NewConfigRetriever(_ context.Context, clock clock.WithTicker, store *store, storeUpdated *bool, isLeader func() bool, rcClient RcClient) (*ConfigRetriever, error) {
	cr := &ConfigRetriever{
		isLeader: isLeader,
		clock:    clock,

		valuesProcessor: newAutoscalingValuesProcessor(store, storeUpdated),
	}

	// Subscribe to remote config updates
	rcClient.SubscribeIgnoreExpiration(data.ProductClusterAutoscalingValues, cr.autoscalingValuesCallback)

	log.Infof("Created new cluster scaling config retriever")
	return cr, nil
}

func (cr *ConfigRetriever) autoscalingValuesCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	cr.valuesProcessor.preProcess()
	for configKey, rawConfig := range updates {
		log.Debugf("Processing config key: %s, product: %s, id: %s, name: %s, version: %d, leader: %v", configKey, rawConfig.Metadata.Product, rawConfig.Metadata.ID, rawConfig.Metadata.Name, rawConfig.Metadata.Version, cr.isLeader())

		err := cr.valuesProcessor.process(configKey, rawConfig)
		if err != nil {
			log.Warnf("Error processing autoscaling values for %s: %v", configKey, err)
			applyStateCallback(configKey, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
		} else {
			applyStateCallback(configKey, state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
				Error: "",
			})
		}
	}

	cr.valuesProcessor.postProcess()
}

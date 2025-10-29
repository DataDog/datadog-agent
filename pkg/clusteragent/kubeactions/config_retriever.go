// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RcClient is a subinterface of rcclient.Component to allow mocking
type RcClient interface {
	SubscribeIgnoreExpiration(product string, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
}

// ConfigRetriever is responsible for retrieving remote config updates for Kubernetes actions
type ConfigRetriever struct {
	processor *ActionProcessor
	isLeader  func() bool
}

// NewConfigRetriever creates a new ConfigRetriever
func NewConfigRetriever(ctx context.Context, processor *ActionProcessor, isLeader func() bool, rcClient RcClient) (*ConfigRetriever, error) {
	cr := &ConfigRetriever{
		processor: processor,
		isLeader:  isLeader,
	}

	// Subscribe to remote config updates
	rcClient.SubscribeIgnoreExpiration(string(data.ProductKubeActions), cr.actionsCallback)

	log.Infof("Created new Kubernetes actions config retriever")
	return cr, nil
}

// actionsCallback is called when remote config updates are received
func (cr *ConfigRetriever) actionsCallback(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	// Only the leader should process actions
	isLeader := cr.isLeader()

	for configKey, rawConfig := range update {
		log.Debugf("Processing config key: %s, product: %s, id: %s, name: %s, version: %d, leader: %v",
			configKey, rawConfig.Metadata.Product, rawConfig.Metadata.ID, rawConfig.Metadata.Name, rawConfig.Metadata.Version, isLeader)

		if !isLeader {
			applyStateCallback(configKey, state.ApplyStatus{
				State: state.ApplyStateUnacknowledged,
				Error: "not the leader",
			})
			continue
		}

		// Process the actions
		err := cr.processor.Process(configKey, rawConfig)
		if err != nil {
			log.Warnf("Error processing Kubernetes actions for %s: %v", configKey, err)
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
}

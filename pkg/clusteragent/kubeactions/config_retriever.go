// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"context"

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
	log.Infof("[KubeActions] Creating new ConfigRetriever...")
	cr := &ConfigRetriever{
		processor: processor,
		isLeader:  isLeader,
	}

	// Subscribe to remote config updates
	// TODO(KUBEACTIONS-POC): CHANGE TO ProductKubeActions - Using DEBUG product for PoC testing
	// Once kubeactions track is created in production, change "DEBUG" to ProductKubeActions
	log.Infof("[KubeActions] Subscribing to DEBUG product for remote config updates...")
	rcClient.SubscribeIgnoreExpiration("DEBUG", cr.actionsCallback)
	log.Infof("[KubeActions] Successfully subscribed to DEBUG product, callback registered")

	log.Infof("[KubeActions] ConfigRetriever created successfully")
	return cr, nil
}

// actionsCallback is called when remote config updates are received
func (cr *ConfigRetriever) actionsCallback(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	// Only the leader should process actions
	isLeader := cr.isLeader()

	// Log every time RC callback is invoked
	log.Infof("[KubeActions] RC callback invoked: received %d config(s), leader=%v", len(update), isLeader)

	if len(update) == 0 {
		log.Infof("[KubeActions] No configs in this RC update")
		return
	}

	for configKey, rawConfig := range update {
		log.Infof("[KubeActions] Processing config key: %s", configKey)
		log.Infof("[KubeActions]   - Product: %s", rawConfig.Metadata.Product)
		log.Infof("[KubeActions]   - ID: %s", rawConfig.Metadata.ID)
		log.Infof("[KubeActions]   - Name: %s", rawConfig.Metadata.Name)
		log.Infof("[KubeActions]   - Version: %d", rawConfig.Metadata.Version)
		log.Infof("[KubeActions]   - Raw data length: %d bytes", len(rawConfig.Config))
		log.Infof("[KubeActions]   - Raw data preview: %s", string(rawConfig.Config[:min(len(rawConfig.Config), 200)]))

		if !isLeader {
			log.Infof("[KubeActions] Skipping config %s - not the leader", configKey)
			applyStateCallback(configKey, state.ApplyStatus{
				State: state.ApplyStateUnacknowledged,
				Error: "not the leader",
			})
			continue
		}

		// Process the actions
		log.Infof("[KubeActions] Starting to process actions for config %s", configKey)
		err := cr.processor.Process(configKey, rawConfig)
		if err != nil {
			log.Errorf("[KubeActions] Error processing actions for %s: %v", configKey, err)
			applyStateCallback(configKey, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
		} else {
			log.Infof("[KubeActions] Successfully processed actions for config %s", configKey)
			applyStateCallback(configKey, state.ApplyStatus{
				State: state.ApplyStateAcknowledged,
				Error: "",
			})
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

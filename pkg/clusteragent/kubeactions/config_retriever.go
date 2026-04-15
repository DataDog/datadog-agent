// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
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

// NewConfigRetriever creates a new ConfigRetriever and subscribes to K8S_ACTIONS
func NewConfigRetriever(processor *ActionProcessor, isLeader func() bool, rcClient RcClient) *ConfigRetriever {
	cr := &ConfigRetriever{
		processor: processor,
		isLeader:  isLeader,
	}

	rcClient.SubscribeIgnoreExpiration(state.ProductK8SActions, cr.actionsCallback)
	log.Infof("[KubeActions] Subscribed to %s remote config product", state.ProductK8SActions)

	return cr
}

// actionsCallback is called when remote config updates are received
func (cr *ConfigRetriever) actionsCallback(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	isLeader := cr.isLeader()

	log.Infof("[KubeActions] RC callback invoked: received %d config(s), leader=%v", len(update), isLeader)

	if len(update) == 0 {
		return
	}

	for configKey, rawConfig := range update {
		log.Infof("[KubeActions] Processing config: key=%s, id=%s, version=%d, size=%d bytes",
			configKey, rawConfig.Metadata.ID, rawConfig.Metadata.Version, len(rawConfig.Config))

		if !isLeader {
			log.Infof("[KubeActions] Skipping config %s - not the leader", configKey)
			applyStateCallback(configKey, state.ApplyStatus{
				State: state.ApplyStateUnacknowledged,
				Error: "not the leader",
			})
			continue
		}

		// ACK immediately on receipt — execution results are reported via EVP
		applyStateCallback(configKey, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})

		err := cr.processor.Process(configKey, rawConfig)
		if err != nil {
			log.Errorf("[KubeActions] Error processing actions for %s: %v", configKey, err)
		} else {
			log.Infof("[KubeActions] Successfully processed actions for config %s", configKey)
		}
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"time"

	"k8s.io/utils/clock"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	configRetrieverStoreID    autoscaling.SenderID = "cr"
	settingsReconcileInterval time.Duration        = 5 * time.Minute
	valuesReconcileInterval   time.Duration        = 5 * time.Minute
)

// RcClient is a subinterface of rcclient.Component to allow mocking
type RcClient interface {
	SubscribeIgnoreExpiration(product string, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
}

// ConfigRetriever is responsible for retrieving remote objects (Autoscaling .Spec and values)
type ConfigRetriever struct {
	isLeader func() bool
	clock    clock.WithTicker

	settingsProcessor autoscalingSettingsProcessor
	valuesProcessor   autoscalingValuesProcessor
}

type autoscalingProcessor interface {
	preProcess()
	processItem(receivedTimestamp time.Time, configKey string, rawConfig state.RawConfig) error
	postProcess()
	reconcile(isLeader bool)
}

// NewConfigRetriever creates a new ConfigRetriever
func NewConfigRetriever(ctx context.Context, clock clock.WithTicker, store *store, isLeader func() bool, rcClient RcClient) (*ConfigRetriever, error) {
	cr := &ConfigRetriever{
		isLeader: isLeader,
		clock:    clock,

		settingsProcessor: newAutoscalingSettingsProcessor(store),
		valuesProcessor:   newAutoscalingValuesProcessor(store),
	}

	// Subscribe to remote config updates
	rcClient.SubscribeIgnoreExpiration(data.ProductContainerAutoscalingSettings, func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
		cr.processorCallback(&cr.settingsProcessor, update, applyStateCallback)
	})
	rcClient.SubscribeIgnoreExpiration(data.ProductContainerAutoscalingValues, func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
		cr.processorCallback(&cr.valuesProcessor, update, applyStateCallback)
	})

	// Add a regular reconcile for settings. Several edge cases can happen that would prevent creation or deletion of a PodAutoscaler
	// For instance, if a leader change happens before the old persisted the update in Kubernetes.
	go func() {
		ticker := cr.clock.NewTicker(settingsReconcileInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C():
				cr.settingsProcessor.reconcile(cr.isLeader())
			}
		}
	}()

	// Add a regular reconcile for values. Similar edge cases can happen with values processing.
	go func() {
		ticker := cr.clock.NewTicker(valuesReconcileInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C():
				cr.valuesProcessor.reconcile(cr.isLeader())
			}
		}
	}()

	log.Debugf("Created new workload scaling config retriever")
	return cr, nil
}

func (cr *ConfigRetriever) processorCallback(processor autoscalingProcessor, update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	timestamp := cr.clock.Now()

	processor.preProcess()
	for configKey, rawConfig := range update {
		log.Debugf("Processing config key: %s, product: %s, id: %s, name: %s, version: %d, leader: %v", configKey, rawConfig.Metadata.Product, rawConfig.Metadata.ID, rawConfig.Metadata.Name, rawConfig.Metadata.Version, cr.isLeader())

		err := processor.processItem(timestamp, configKey, rawConfig)
		if err != nil {
			log.Warnf("Error processing item from product %s for config key %s: %v", rawConfig.Metadata.Product, configKey, err)
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
	processor.postProcess()

	// Reconcile the remote config state and the local store
	processor.reconcile(cr.isLeader())
}

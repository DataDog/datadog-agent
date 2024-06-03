// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package impl

import (
	"time"

	"k8s.io/utils/clock"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	configRetrieverStoreID string = "cr"
)

// rcClient is a subinterface of rcclient.Component to allow mocking
type rcClient interface {
	Subscribe(product string, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
}

// configRetriever is responsible for retrieving remote objects (Autoscaling .Spec and values)
type configRetriever struct {
	store    *store
	isLeader func() bool
	clock    clock.Clock
}

func newConfigRetriever(store *store, isLeader func() bool, rcClient rcClient) (*configRetriever, error) {
	cr := &configRetriever{
		store:    store,
		isLeader: isLeader,
		clock:    clock.RealClock{},
	}

	rcClient.Subscribe(data.ProductContainerAutoscalingSettings, func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
		// For autoscaling settings, we need to be able to clean up the store to handle deleted configs.
		// Remote config guarantees that we receive all configs at once, so we can safely clean up the store after processing all configs.
		autoscalingSettingsProcessor := newAutoscalingSettingsProcessor(cr.store)
		cr.autoscalerUpdateCallback(cr.clock.Now(), update, applyStateCallback, autoscalingSettingsProcessor.process, autoscalingSettingsProcessor.postProcess)
	})

	rcClient.Subscribe(data.ProductContainerAutoscalingValues, func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
		autoscalingValuesProcessor := newAutoscalingValuesProcessor(cr.store)
		cr.autoscalerUpdateCallback(cr.clock.Now(), update, applyStateCallback, autoscalingValuesProcessor.process, autoscalingValuesProcessor.postProcess)
	})

	log.Debugf("Created new workload scaling config retriever")
	return cr, nil
}

func (cr *configRetriever) autoscalerUpdateCallback(
	timestamp time.Time,
	update map[string]state.RawConfig,
	applyStateCallback func(string, state.ApplyStatus),
	process func(time.Time, string, state.RawConfig) error,
	postProcess func(errors []error),
) {
	log.Tracef("Received update from RC")

	isLeader := cr.isLeader()
	var errors []error
	for configKey, rawConfig := range update {
		log.Debugf("Processing config key: %s, product: %s, id: %s, name: %s, version: %d, leader: %v", configKey, rawConfig.Metadata.Product, rawConfig.Metadata.ID, rawConfig.Metadata.Name, rawConfig.Metadata.Version, isLeader)

		if !isLeader {
			applyStateCallback(configKey, state.ApplyStatus{
				State: state.ApplyStateUnacknowledged,
				Error: "",
			})
			continue
		}

		err := process(timestamp, configKey, rawConfig)
		if err != nil {
			errors = append(errors, err)
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

	// If `process` was not called, we're not calling postProcess
	if isLeader && postProcess != nil {
		postProcess(errors)
	}
}

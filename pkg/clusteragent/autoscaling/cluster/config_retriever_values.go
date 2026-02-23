// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"encoding/json"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type autoscalingValuesProcessor struct {
	store               *store
	storeUpdated        *bool
	processed           map[string]struct{}
	lastProcessingError bool
}

func newAutoscalingValuesProcessor(store *store, storeUpdated *bool) autoscalingValuesProcessor {
	return autoscalingValuesProcessor{
		store:        store,
		storeUpdated: storeUpdated,
	}
}

func (avp *autoscalingValuesProcessor) preProcess() {
	avp.processed = make(map[string]struct{}, len(avp.processed))
	avp.lastProcessingError = false
}

func (avp *autoscalingValuesProcessor) process(configKey string, rawConfig state.RawConfig) error {
	valuesList := &kubeAutoscaling.ClusterAutoscalingValuesList{}
	err := json.Unmarshal(rawConfig.Config, &valuesList)
	if err != nil {
		avp.lastProcessingError = true
		log.Errorf("failed to unmarshal config id:%s, version: %d, config key: %s, err: %v", rawConfig.Metadata.ID, rawConfig.Metadata.Version, configKey, err)
		return err
	}

	for _, values := range valuesList.Values {
		avp.processValues(values, rawConfig.Metadata.Version)
	}

	return nil
}

func (avp *autoscalingValuesProcessor) processValues(values *kubeAutoscaling.ClusterAutoscalingValues, _ uint64) {
	npi := model.NewNodePoolInternal(values)

	id := values.Name
	avp.processed[id] = struct{}{}
	avp.store.Set(id, npi, configRetrieverStoreID)
}

func (avp *autoscalingValuesProcessor) postProcess() {
	// Don't delete configs if we received incorrect data
	if avp.lastProcessingError {
		log.Debug("Skipping autoscaling values clean up due to errors while processing new data")
		return
	}

	storeObjects := avp.store.GetAll()

	// Clear values for all configs that are no longer received from Remote Config
	for _, s := range storeObjects {
		if _, found := avp.processed[s.Name()]; !found {
			avp.store.Delete(s.Name(), configRetrieverStoreID)
			log.Debugf("Deleting object from store: %s", s.Name())
		}
	}

	*avp.storeUpdated = true
}

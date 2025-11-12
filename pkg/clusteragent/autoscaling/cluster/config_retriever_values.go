// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"encoding/json"
	// "strconv"
	// "sync"
	// "time"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	// corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type valuesItem struct {
	name string
	// receivedTimestamp time.Time
	receivedVersion  string
	nodePoolInternal model.NodePoolInternal
}

type autoscalingValuesProcessor struct {
	store        *store
	storeUpdated *bool
	processed    map[string]struct{}
	// state        map[string]valuesItem
	// updateLock sync.Mutex

	// newState            map[string]valuesItem
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
	// avp.newState = make(map[string]valuesItem, len(avp.state))
	// avp.updateLock.Lock()
}

func (avp *autoscalingValuesProcessor) process(configKey string, rawConfig state.RawConfig) error {
	valuesList := &kubeAutoscaling.ClusterAutoscalingValuesList{}
	err := json.Unmarshal(rawConfig.Config, &valuesList)
	if err != nil {
		avp.lastProcessingError = true
		log.Errorf("failed to unmarshal config id:%s, version: %d, config key: %s, err: %v", rawConfig.Metadata.ID, rawConfig.Metadata.Version, configKey, err)
		return err
	}

	// Store minNodePools from remote config payload
	for _, values := range valuesList.Values {
		err = avp.processValues(values, rawConfig.Metadata.Version)

		// id := values.Name
		// avp.processed[id] = struct{}{}
		// avp.store.Set(id, mnp, configRetrieverStoreID)
	}

	avp.lastProcessingError = err != nil
	return err
}

func (avp *autoscalingValuesProcessor) processValues(values *kubeAutoscaling.ClusterAutoscalingValues, receivedVersion uint64) error {
	npi := model.NewNodePoolInternal(values)

	id := values.Name
	avp.processed[id] = struct{}{}
	avp.store.Set(id, npi, configRetrieverStoreID)

	// Buffer the values in newState instead of updating store directly
	// Existence checks and custom recommender config checks will be done during reconcile
	// avp.newState[id] = valuesItem{
	// 	name: values.Name,
	// 	// receivedTimestamp: timestamp,
	// 	receivedVersion:  strconv.FormatUint(receivedVersion, 10),
	// 	nodePoolInternal: npi,
	// }

	// log.Infof("CELENE inside processValues, newState updated with id %s", id)
	log.Infof("CELENE inside processValues, store updated with id %s", id)

	// CELENE WILL THIS EVER RETURN AN ERROR?
	return nil
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

	// avp.state = avp.newState
	// avp.newState = nil
	// avp.updateLock.Unlock()
}

// func (avp *autoscalingValuesProcessor) reconcile(isLeader bool) {
// 	log.Infof("CELENE inside reconcile")

// 	// We only reconcile if we are the leader and we have a state
// 	if !isLeader || avp.state == nil {
// 		return
// 	}

// 	// If we cannot TryLock, it means an update is already running.
// 	// It would be useless to reconcile right after as no new data would have been received
// 	if !avp.updateLock.TryLock() {
// 		return
// 	}

// 	defer avp.updateLock.Unlock()

// 	// CELENE should i be checking for receivedVersion? in workload values, we just use it for telemetry
// 	for id, item := range avp.state {
// 		// av, avFound := avp.store.LockRead(id, false) // CELENE in our case i don't think we need to read store actually, because RC is source of truth

// 		// if !avFound {
// 		// 	continue
// 		// }

// 		avp.store.Set(id, item.nodePoolInternal, configRetrieverStoreID)
// 		log.Infof("CELENE inside reconcile, store should be updated")
// 	}

// 	// Clear values for all configs that are no longer received from Remote Config
// 	storeObjects := avp.store.GetAll()
// 	for _, s := range storeObjects {
// 		if _, found := avp.state[s.Name]; !found {
// 			avp.store.Delete(s.Name, configRetrieverStoreID)
// 			log.Infof("CELENE inside reconcile, deleting nodepool %s", s.Name)
// 			log.Debugf("Deleting object from store: %s", s.Name)
// 		}
// 	}
// 	*avp.storeUpdated = true
// 	log.Infof("CELENE inside reconcile, storeUpdated: %v", *avp.storeUpdated)

// }

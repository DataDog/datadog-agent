// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workloadimpl

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/comp/autoscaling/workload/impl/model"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
)

type autoscalingSettingsProcessor struct {
	store     *store
	processed map[string]struct{}
}

func newAutoscalingSettingsProcessor(store *store) autoscalingSettingsProcessor {
	return autoscalingSettingsProcessor{
		store:     store,
		processed: make(map[string]struct{}),
	}
}

func (p autoscalingSettingsProcessor) process(receivedTimestamp time.Time, configKey string, rawConfig state.RawConfig) error {
	settingsList := &model.AutoscalingSettingsList{}
	err := json.Unmarshal(rawConfig.Config, &settingsList)
	if err != nil {
		return fmt.Errorf("failed to unmarshal config id:%s, version: %d, config key: %s, err: %v", rawConfig.Metadata.ID, rawConfig.Metadata.Version, configKey, err)
	}

	// Creating/Updating received PodAutoscalers
	for _, settings := range settingsList.Settings {
		if settings.Namespace == "" || settings.Name == "" || settings.Spec == nil {
			err = multierror.Append(err, fmt.Errorf("received invalid PodAutoscaler from config id:%s, version: %d, config key: %s, discarding", rawConfig.Metadata.ID, rawConfig.Metadata.Version, configKey))
		}

		podAutoscalerID := autoscaling.BuildObjectID(settings.Namespace, settings.Name)
		podAutoscaler, podAutoscalerFound := p.store.LockRead(podAutoscalerID, true)
		// If the PodAutoscaler is not found, we need to create it
		if !podAutoscalerFound {
			podAutoscaler = model.PodAutoscalerInternal{
				Namespace: settings.Namespace,
				Name:      settings.Name,
			}
		}

		podAutoscaler.UpdateFromSettings(settings.Spec, rawConfig.Metadata.Version, receivedTimestamp)
		p.store.UnlockSet(podAutoscalerID, podAutoscaler, configRetrieverStoreID)
		p.processed[podAutoscalerID] = struct{}{}
	}

	return err
}

// postProcess is used after all configs have been processed to clear internal store from missing configs
func (p autoscalingSettingsProcessor) postProcess(errors []error) {
	// We don't want to delete configs if we received incorrect data
	if len(errors) > 0 {
		log.Debugf("Skipping autoscaling settings clean up due to errors while processing new data: %v", errors)
		return
	}

	// We first get all PodAutoscalers that were not part of the last update
	var toDelete []string
	_ = p.store.GetFiltered(func(pai model.PodAutoscalerInternal) bool {
		if pai.Spec == nil || pai.Spec.Owner != v1alpha1.DatadogPodAutoscalerRemoteOwner {
			return false
		}

		_, found := p.processed[pai.ID()]
		if !found {
			toDelete = append(toDelete, pai.ID())
		}

		return false
	})

	// Then we can properly lock read/write to flag them as deleted
	for _, id := range toDelete {
		pai, found := p.store.LockRead(id, false)
		if found {
			pai.Deleted = true
			log.Infof("PodAutoscaler %s was not part of the last update, flagging it as deleted", id)
			p.store.UnlockSet(id, pai, configRetrieverStoreID)
		}
	}
}

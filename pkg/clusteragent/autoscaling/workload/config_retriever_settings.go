// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// This is used to make sure `RemoteVersion` field in CRD .Spec reflect the working version of the CRD
// It will allow the Cluster Agent to auto-upgrade previous versions of the CRD to the current one
const (
	// Current version is v1alpha2, add 1e9 for a new version
	versionOffset uint64 = 1e9
)

type settingsItem struct {
	namespace         string
	name              string
	receivedTimestamp time.Time
	spec              *datadoghq.DatadogPodAutoscalerSpec
}

type autoscalingSettingsProcessor struct {
	store *store
	// State is kept nil until the first full config is processed
	state map[string]settingsItem
	// We are guaranteed to be called in a single thread for pre/process/post
	// However, reconcile could be called in parallel
	updateLock sync.Mutex

	newState            map[string]settingsItem
	lastProcessingError bool
}

func newAutoscalingSettingsProcessor(store *store) autoscalingSettingsProcessor {
	return autoscalingSettingsProcessor{
		store: store,
	}
}

func (p *autoscalingSettingsProcessor) preProcess() {
	p.lastProcessingError = false
	p.newState = make(map[string]settingsItem, len(p.state))
	p.updateLock.Lock()
}

func (p *autoscalingSettingsProcessor) processItem(receivedTimestamp time.Time, configKey string, rawConfig state.RawConfig) error {
	settingsList := &model.AutoscalingSettingsList{}
	err := json.Unmarshal(rawConfig.Config, &settingsList)
	if err != nil {
		p.lastProcessingError = true
		return fmt.Errorf("failed to unmarshal config id:%s, version: %d, config key: %s, err: %v", rawConfig.Metadata.ID, rawConfig.Metadata.Version, configKey, err)
	}

	// Creating/Updating received PodAutoscalers
	for _, settings := range settingsList.Settings {
		// Resolve/Convert .spec to expected version
		spec := extractConvertAutoscalerSpec(settings)

		if settings.Namespace == "" || settings.Name == "" || spec == nil {
			err = multierror.Append(err, fmt.Errorf("received invalid PodAutoscaler from config id:%s, version: %d, config key: %s, discarding", rawConfig.Metadata.ID, rawConfig.Metadata.Version, configKey))
		}

		podAutoscalerID := autoscaling.BuildObjectID(settings.Namespace, settings.Name)
		spec.RemoteVersion = pointer.Ptr(versionOffset + rawConfig.Metadata.Version)
		p.newState[podAutoscalerID] = settingsItem{
			namespace:         settings.Namespace,
			name:              settings.Name,
			receivedTimestamp: receivedTimestamp,
			spec:              spec,
		}
	}

	if err != nil {
		p.lastProcessingError = true
	}
	return err
}

// postProcess is used after all configs have been processed to clear internal store from missing configs
func (p *autoscalingSettingsProcessor) postProcess() {
	// TODO: How to handle the case where the remote version is lower than the local version?
	// It can happen in case of file split
	p.state = p.newState
	p.newState = nil
	p.updateLock.Unlock()
}

func (p *autoscalingSettingsProcessor) reconcile(isLeader bool) {
	// We only reconcile if we are the leader and we have a state
	if !isLeader || p.state == nil {
		return
	}

	// If we cannot TryLock, it means an update is already running.
	// It would be useless to reconcile right after as no new data would have been received
	if !p.updateLock.TryLock() {
		return
	}
	defer p.updateLock.Unlock()

	inStore := make(map[string]struct{}, len(p.state))

	// Handle the existing and deleted PodAutoscalers
	p.store.Update(func(pai model.PodAutoscalerInternal) (model.PodAutoscalerInternal, bool) {
		if pai.Spec() == nil || pai.Spec().Owner != datadoghqcommon.DatadogPodAutoscalerRemoteOwner {
			return pai, false
		}

		paID := pai.ID()
		inStore[paID] = struct{}{}

		settingsItem, found := p.state[paID]
		if found {
			pai.UpdateFromSettings(settingsItem.spec, settingsItem.receivedTimestamp)
			return pai, true
		}

		// Not found in the new state, marking for deletion if no error occurred while processing new data
		if !p.lastProcessingError {
			pai.SetDeleted()
			log.Infof("PodAutoscaler %s was not part of the last update, flagging it as deleted", paID)
			return pai, true
		}

		log.Debugf("PodAutoscaler %s was not part of the last update, but we skipped the deletion due to errors while processing new data", paID)
		return pai, false
	}, configRetrieverStoreID)

	// Handle the potentially new PodAutoscalers, note that there is a chance they have been created since the `Update` call above
	for paID, item := range p.state {
		if _, found := inStore[paID]; !found {
			podAutoscaler, podAutoscalerFound := p.store.LockRead(paID, true)
			if podAutoscalerFound {
				podAutoscaler.UpdateFromSettings(item.spec, item.receivedTimestamp)
			} else {
				podAutoscaler = model.NewPodAutoscalerFromSettings(item.namespace, item.name, item.spec, item.receivedTimestamp)
			}
			p.store.UnlockSet(paID, podAutoscaler, configRetrieverStoreID)
		}
	}
}

func extractConvertAutoscalerSpec(settings model.AutoscalingSettings) *datadoghq.DatadogPodAutoscalerSpec {
	if settings.Specs != nil {
		if settings.Specs.V1Alpha2 != nil {
			return settings.Specs.V1Alpha2
		}

		if settings.Specs.V1Alpha1 != nil {
			return pointer.Ptr(datadoghq.ConvertDatadogPodAutoscalerSpecFromV1Alpha1(*settings.Specs.V1Alpha1))
		}
	}

	if settings.Spec != nil {
		return pointer.Ptr(datadoghq.ConvertDatadogPodAutoscalerSpecFromV1Alpha1(*settings.Spec))
	}

	return nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"encoding/json"
	"fmt"
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
		// Resolve/Convert .spec to expected version
		spec := extractConvertAutoscalerSpec(settings)

		if settings.Namespace == "" || settings.Name == "" || spec == nil {
			err = multierror.Append(err, fmt.Errorf("received invalid PodAutoscaler from config id:%s, version: %d, config key: %s, discarding", rawConfig.Metadata.ID, rawConfig.Metadata.Version, configKey))
		}

		podAutoscalerRemoteVersion := versionOffset + rawConfig.Metadata.Version
		podAutoscalerID := autoscaling.BuildObjectID(settings.Namespace, settings.Name)
		podAutoscaler, podAutoscalerFound := p.store.LockRead(podAutoscalerID, true)
		// If the PodAutoscaler is not found, we need to create it
		if !podAutoscalerFound {
			podAutoscaler = model.NewPodAutoscalerFromSettings(settings.Namespace, settings.Name, spec, podAutoscalerRemoteVersion, receivedTimestamp)
		} else {
			podAutoscaler.UpdateFromSettings(spec, podAutoscalerRemoteVersion, receivedTimestamp)
		}

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

	// Update the store to flag all PodAutoscalers owned by remote that were not processed
	p.store.Update(func(pai model.PodAutoscalerInternal) (model.PodAutoscalerInternal, bool) {
		if pai.Spec() == nil || pai.Spec().Owner != datadoghqcommon.DatadogPodAutoscalerRemoteOwner {
			return pai, false
		}

		paID := pai.ID()
		_, found := p.processed[paID]
		if !found {
			pai.SetDeleted()
			log.Infof("PodAutoscaler %s was not part of the last update, flagging it as deleted", paID)
			return pai, true
		}

		return pai, false
	}, configRetrieverStoreID)
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

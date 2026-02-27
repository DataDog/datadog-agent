// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/hashicorp/go-multierror"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type valuesItem struct {
	namespace         string
	name              string
	receivedTimestamp time.Time
	receivedVersion   uint64
	scalingValues     model.ScalingValues
}

type autoscalingValuesProcessor struct {
	store *store
	// State is kept nil until the first full config is processed
	state map[string]valuesItem
	// We are guaranteed to be called in a single thread for pre/process/post
	// However, reconcile could be called in parallel
	updateLock sync.Mutex

	newState            map[string]valuesItem
	lastProcessingError bool
}

func newAutoscalingValuesProcessor(store *store) autoscalingValuesProcessor {
	return autoscalingValuesProcessor{
		store: store,
	}
}

func (p *autoscalingValuesProcessor) preProcess() {
	p.lastProcessingError = false
	p.newState = make(map[string]valuesItem, len(p.state))
	p.updateLock.Lock()
}

func (p *autoscalingValuesProcessor) processItem(receivedTimestamp time.Time, configKey string, rawConfig state.RawConfig) error {
	valuesList := &kubeAutoscaling.WorkloadValuesList{}
	err := json.Unmarshal(rawConfig.Config, &valuesList)
	if err != nil {
		p.lastProcessingError = true
		return fmt.Errorf("failed to unmarshal config id:%s, version: %d, config key: %s, err: %v", rawConfig.Metadata.ID, rawConfig.Metadata.Version, configKey, err)
	}

	for _, values := range valuesList.Values {
		processErr := p.processValues(values, rawConfig.Metadata.Version, receivedTimestamp)
		if processErr != nil {
			err = multierror.Append(err, fmt.Errorf("received invalid Autoscaling Values from config id:%s, version: %d, config key: %s, discarding", rawConfig.Metadata.ID, rawConfig.Metadata.Version, configKey))
		}
	}

	p.lastProcessingError = err != nil
	return err
}

func (p *autoscalingValuesProcessor) processValues(values *kubeAutoscaling.WorkloadValues, receivedVersion uint64, timestamp time.Time) error {
	if values == nil || values.Namespace == "" || values.Name == "" {
		// Should never happen, but protecting the code from invalid inputs
		return nil
	}

	id := autoscaling.BuildObjectID(values.Namespace, values.Name)

	scalingValues, err := parseAutoscalingValues(timestamp, values)
	if err != nil {
		return fmt.Errorf("failed to parse scaling values for PodAutoscaler %s: %w", id, err)
	}

	// Buffer the values in newState instead of updating store directly
	// Existence checks and custom recommender config checks will be done during reconcile
	p.newState[id] = valuesItem{
		namespace:         values.Namespace,
		name:              values.Name,
		receivedTimestamp: timestamp,
		receivedVersion:   receivedVersion,
		scalingValues:     scalingValues,
	}

	return nil
}

// postProcess is used after all configs have been processed to update internal state
func (p *autoscalingValuesProcessor) postProcess() {
	// TODO: How to handle the case where the remote version is lower than the local version?
	// It can happen in case of file split
	p.state = p.newState
	p.newState = nil
	p.updateLock.Unlock()
}

func (p *autoscalingValuesProcessor) reconcile(isLeader bool) {
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

	// Update PodAutoscalers with buffered values
	for paID, item := range p.state {
		podAutoscaler, podAutoscalerFound := p.store.LockRead(paID, false)
		// If the PodAutoscaler is not found, it must be created through the controller
		// discarding the values received here.
		// The store is not locked as we call LockRead with lockOnMissing = false
		if !podAutoscalerFound {
			continue
		}

		// Ignore values if the PodAutoscaler has a custom recommender configuration
		if podAutoscaler.CustomRecommenderConfiguration() != nil {
			p.store.UnlockSet(paID, podAutoscaler, configRetrieverStoreID)
			continue
		}

		// Update PodAutoscaler values with received values
		podAutoscaler.UpdateFromMainValues(item.scalingValues, item.receivedVersion)

		p.store.UnlockSet(paID, podAutoscaler, configRetrieverStoreID)
	}

	// Clear values for all configs that were removed (only if no error occurred while processing new data)
	if !p.lastProcessingError {
		p.store.Update(func(podAutoscaler model.PodAutoscalerInternal) (model.PodAutoscalerInternal, bool) {
			if _, found := p.state[podAutoscaler.ID()]; !found {
				log.Infof("Autoscaling not present from remote values, removing values for PodAutoscaler %s", podAutoscaler.ID())
				podAutoscaler.RemoveMainValues()
				return podAutoscaler, true
			}

			return podAutoscaler, false
		}, configRetrieverStoreID)
	} else {
		log.Debugf("Skipping autoscaling values clean up due to errors while processing new data")
	}
}

func parseAutoscalingValues(timestamp time.Time, values *kubeAutoscaling.WorkloadValues) (model.ScalingValues, error) {
	scalingValues := model.ScalingValues{}
	if values.Error != nil {
		scalingValues.Error = (*model.ReccomendationError)(values.Error)
	}

	// Priority is implemented the same way for Horizontal and Vertical scaling
	// Manual values > Auto values
	if values.Horizontal != nil {
		if values.Horizontal.Error != nil {
			scalingValues.HorizontalError = (*model.ReccomendationError)(values.Horizontal.Error)
		}

		var err error
		if values.Horizontal.Manual != nil {
			scalingValues.Horizontal, err = parseHorizontalScalingData(timestamp, values.Horizontal.Manual, datadoghqcommon.DatadogPodAutoscalerManualValueSource)
		} else if values.Horizontal.Auto != nil {
			scalingValues.Horizontal, err = parseHorizontalScalingData(timestamp, values.Horizontal.Auto, datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource)
		}

		if err != nil {
			return model.ScalingValues{}, err
		}
	}

	if values.Vertical != nil {
		if values.Vertical.Error != nil {
			scalingValues.VerticalError = (*model.ReccomendationError)(values.Vertical.Error)
		}

		var err error
		if values.Vertical.Manual != nil {
			scalingValues.Vertical, err = parseAutoscalingVerticalData(timestamp, values.Vertical.Manual, datadoghqcommon.DatadogPodAutoscalerManualValueSource)
		} else if values.Vertical.Auto != nil {
			scalingValues.Vertical, err = parseAutoscalingVerticalData(timestamp, values.Vertical.Auto, datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource)
		}

		if err != nil {
			return model.ScalingValues{}, err
		}
	}

	return scalingValues, nil
}

func parseHorizontalScalingData(timestamp time.Time, data *kubeAutoscaling.WorkloadHorizontalData, source datadoghqcommon.DatadogPodAutoscalerValueSource) (*model.HorizontalScalingValues, error) {
	horizontalValues := &model.HorizontalScalingValues{
		Source: source,
	}

	if data.Timestamp != nil {
		horizontalValues.Timestamp = data.Timestamp.AsTime()
	} else {
		// We default to the received timestamp if the value is not set from the backend, should not happen
		// TODO: Remove when backend has been updated and return an error instead
		horizontalValues.Timestamp = timestamp
	}

	if data.Replicas != nil {
		horizontalValues.Replicas = *data.Replicas
	} else {
		return nil, errors.New("horizontal replicas value are missing")
	}

	return horizontalValues, nil
}

func parseAutoscalingVerticalData(timestamp time.Time, data *kubeAutoscaling.WorkloadVerticalData, source datadoghqcommon.DatadogPodAutoscalerValueSource) (*model.VerticalScalingValues, error) {
	verticalValues := &model.VerticalScalingValues{
		Source: source,
	}

	if data.Timestamp != nil {
		verticalValues.Timestamp = data.Timestamp.AsTime()
	} else {
		// We default to the received timestamp if the value is not set from the backend, should not happen
		// TODO: Remove when backend has been updated and return an error instead
		verticalValues.Timestamp = timestamp
	}

	if containersNum := len(data.Resources); containersNum > 0 {
		verticalValues.ContainerResources = make([]datadoghqcommon.DatadogPodAutoscalerContainerResources, 0, containersNum)

		for _, containerResources := range data.Resources {
			convertedResources := datadoghqcommon.DatadogPodAutoscalerContainerResources{
				Name: containerResources.ContainerName,
			}

			if limits, err := parseResourceList(containerResources.Limits); err == nil {
				convertedResources.Limits = limits
			} else {
				return nil, err
			}

			if requests, err := parseResourceList(containerResources.Requests); err == nil {
				convertedResources.Requests = requests
			} else {
				return nil, err
			}

			// Validating that requests are <= limits
			for resourceName, requestQty := range convertedResources.Requests {
				if limitQty, found := convertedResources.Limits[resourceName]; found && limitQty.Cmp(requestQty) < 0 {
					return nil, fmt.Errorf("resource: %s, request %s is greater than limit %s", resourceName, requestQty.String(), limitQty.String())
				}
			}

			verticalValues.ContainerResources = append(verticalValues.ContainerResources, convertedResources)
		}
	}

	var err error
	verticalValues.ResourcesHash, err = autoscaling.ObjectHash(verticalValues.ContainerResources)
	if err != nil {
		return nil, fmt.Errorf("failed to hash container resources: %w", err)
	}

	return verticalValues, nil
}

func parseResourceList(resourceList []*kubeAutoscaling.ContainerResources_ResourceList) (corev1.ResourceList, error) {
	if resourceList == nil {
		return nil, nil
	}

	corev1ResourceList := make(corev1.ResourceList, len(resourceList))
	for _, containerResource := range resourceList {
		if _, found := corev1ResourceList[corev1.ResourceName(containerResource.Name)]; found {
			return nil, fmt.Errorf("resource %s is duplicated", containerResource.Name)
		}

		qty, err := resource.ParseQuantity(containerResource.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to parse resource %s value %s: %w", containerResource.Name, containerResource.Value, err)
		}

		corev1ResourceList[corev1.ResourceName(containerResource.Name)] = qty
	}

	return corev1ResourceList, nil
}

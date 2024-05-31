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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type autoscalingValuesProcessor struct {
	store     *store
	processed map[string]struct{}
}

func newAutoscalingValuesProcessor(store *store) autoscalingValuesProcessor {
	return autoscalingValuesProcessor{
		store:     store,
		processed: make(map[string]struct{}),
	}
}

func (p autoscalingValuesProcessor) process(receivedTimestamp time.Time, configKey string, rawConfig state.RawConfig) error {
	valuesList := &kubeAutoscaling.WorkloadValuesList{}
	err := json.Unmarshal(rawConfig.Config, &valuesList)
	if err != nil {
		return fmt.Errorf("failed to unmarshal config id:%s, version: %d, config key: %s, err: %v", rawConfig.Metadata.ID, rawConfig.Metadata.Version, configKey, err)
	}

	for _, values := range valuesList.Values {
		processErr := p.processValues(values, receivedTimestamp)
		if processErr != nil {
			err = multierror.Append(err, processErr)
		}
	}

	return err
}

func (p autoscalingValuesProcessor) processValues(values *kubeAutoscaling.WorkloadValues, timestamp time.Time) error {
	if values == nil || values.Namespace == "" || values.Name == "" {
		// Should never happen, but protecting the code from invalid inputs
		return nil
	}

	id := autoscaling.BuildObjectID(values.Namespace, values.Name)
	podAutoscaler, podAutoscalerFound := p.store.LockRead(id, false)
	// If the PodAutoscaler is not found, it must be created through the controller
	// discarding the values received here.
	// The store is not locked as we call LockRead with lockOnMissing = false
	if !podAutoscalerFound {
		return nil
	}

	// Update PodAutoscaler values with received values
	// Even on error, the PodAutoscaler can be partially updated, always setting it
	defer func() {
		p.processed[id] = struct{}{}
		p.store.UnlockSet(id, podAutoscaler, configRetrieverStoreID)
	}()
	scalingValues, err := parseAutoscalingValues(timestamp, values)
	if err != nil {
		return fmt.Errorf("failed to parse scaling values for PodAutoscaler %s: %w", id, err)
	}

	podAutoscaler.UpdateFromValues(scalingValues)
	return nil
}

func (p autoscalingValuesProcessor) postProcess(errors []error) {
	// We don't want to delete configs if we received incorrect data
	if len(errors) > 0 {
		return
	}

	// Clear values for all configs that were removed
	p.store.Update(func(podAutoscaler model.PodAutoscalerInternal) (model.PodAutoscalerInternal, bool) {
		if _, found := p.processed[autoscaling.BuildObjectID(podAutoscaler.Namespace, podAutoscaler.Name)]; !found {
			log.Infof("Autoscaling not present from remote values, removing values for PodAutoscaler %s", podAutoscaler.ID())
			podAutoscaler.RemoveValues()
			return podAutoscaler, true
		}

		return podAutoscaler, false
	}, configRetrieverStoreID)
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
			scalingValues.Horizontal, err = parseHorizontalScalingData(timestamp, values.Horizontal.Manual, datadoghq.DatadogPodAutoscalerManualValueSource)
		} else if values.Horizontal.Auto != nil {
			scalingValues.Horizontal, err = parseHorizontalScalingData(timestamp, values.Horizontal.Auto, datadoghq.DatadogPodAutoscalerAutoscalingValueSource)
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
			scalingValues.Vertical, err = parseAutoscalingVerticalData(timestamp, values.Vertical.Manual, datadoghq.DatadogPodAutoscalerManualValueSource)
		} else if values.Vertical.Auto != nil {
			scalingValues.Vertical, err = parseAutoscalingVerticalData(timestamp, values.Vertical.Auto, datadoghq.DatadogPodAutoscalerAutoscalingValueSource)
		}

		if err != nil {
			return model.ScalingValues{}, err
		}
	}

	return scalingValues, nil
}

func parseHorizontalScalingData(timestamp time.Time, data *kubeAutoscaling.WorkloadHorizontalData, source datadoghq.DatadogPodAutoscalerValueSource) (*model.HorizontalScalingValues, error) {
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
		return nil, fmt.Errorf("horizontal replicas value are missing")
	}

	return horizontalValues, nil
}

func parseAutoscalingVerticalData(timestamp time.Time, data *kubeAutoscaling.WorkloadVerticalData, source datadoghq.DatadogPodAutoscalerValueSource) (*model.VerticalScalingValues, error) {
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
		verticalValues.ContainerResources = make([]datadoghq.DatadogPodAutoscalerContainerResources, 0, containersNum)

		for _, containerResources := range data.Resources {
			convertedResources := datadoghq.DatadogPodAutoscalerContainerResources{
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

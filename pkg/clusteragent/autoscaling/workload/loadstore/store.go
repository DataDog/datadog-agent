// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package loadstore

import (
	"strings"

	"github.com/DataDog/agent-payload/v5/gogen"
)

// StoreInfo represents the store information, like memory usage and entity count.
type StoreInfo struct {
	TotalEntityCount        uint64
	currentTime             Timestamp
	EntityStatsByLoadName   map[string]StatsResult
	EntityStatsByNamespace  map[string]StatsResult
	EntityStatsByDeployment map[string]StatsResult
}

type StatsResult struct {
	Count  uint64
	Min    ValueType
	P10    ValueType
	Medium ValueType
	Avg    ValueType
	P95    ValueType
	P99    ValueType
	Max    ValueType
}

// Store is an interface for in-memory storage of entities and their load metric values.
type Store interface {
	// SetEntitiesValues sets the values for the given map
	SetEntitiesValues(entities map[*Entity]*EntityValue)

	// GetStoreInfo returns the store information.
	GetStoreInfo() StoreInfo

	// GetEntitiesStatsByNamespace to get entities stats by namespace
	GetEntitiesStatsByNamespace(namespace string) StatsResult

	// GetEntitiesStatsByLoadName to get entities stats by load load name
	GetEntitiesStatsByLoadName(loadName string) StatsResult

	//DeleteEntityByHashKey to delete entity by hash key
	DeleteEntityByHashKey(hash uint64)
}

// createEntitiesFromPayload is a helper function used for creating entities from the metric payload.
func createEntitiesFromPayload(payload *gogen.MetricPayload) map[*Entity]*EntityValue {
	entities := make(map[*Entity]*EntityValue)
	splitTag := func(tag string) (key string, value string) {
		split := strings.SplitN(tag, ":", 2)
		if len(split) < 2 || split[0] == "" || split[1] == "" {
			return "", ""
		}
		return split[0], split[1]
	}
	for _, series := range payload.Series {
		metricName := series.GetMetric()
		points := series.GetPoints()
		tags := series.GetTags()
		resources := series.GetResources()
		entity := Entity{
			EntityType: UnknownType,
			SourceID:   "",
			Host:       "",
			EntityName: "",
			Namespace:  "",
			LoadName:   metricName,
			Deployment: "",
		}
		for _, resource := range resources {
			if resource.Type == "host" {
				entity.Host = resource.Name
			}
		}
		for _, tag := range tags {
			k, v := splitTag(tag)
			switch k {
			case "display_container_name":
				entity.EntityName = v
			case "kube_namespace":
				entity.Namespace = v
			case "container_id":
				entity.SourceID = v
				entity.EntityType = ContainerType
			case "kube_deployment":
				entity.Deployment = v
			}
		}
		if entity.LoadName == "" || entity.Host == "" || entity.EntityType == UnknownType || entity.Namespace == "" || entity.SourceID == "" {
			continue
		}
		for _, point := range points {
			if point != nil && point.GetTimestamp() > 0 {
				entities[&entity] = &EntityValue{
					value:     ValueType(point.GetValue()),
					timestamp: Timestamp(point.GetTimestamp()),
				}
			}
		}
	}
	return entities
}

// ProcessLoadPayload converts the metric payload and stores the entities and their values in the store.
func ProcessLoadPayload(payload *gogen.MetricPayload, store Store) {
	if payload == nil || store == nil {
		return
	}
	entities := createEntitiesFromPayload(payload)
	store.SetEntitiesValues(entities)
	payload = nil

}

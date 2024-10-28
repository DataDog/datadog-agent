// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package loadstore

import (
	"math"
	"strings"

	"github.com/DataDog/agent-payload/v5/gogen"
)

// Store is an interface for in-memory storage of entities and their load metric values.
type Store interface {
	// SetEntitiesValues sets the values for the given map
	SetEntitiesValues(entities map[*Entity]*EntityValue)

	// GetStoreInfo returns the store information.
	GetStoreInfo() string

	// GetEntitiesByNamespace to get all entities and values by namespace
	GetEntitiesByNamespace(namespace string) map[*Entity]*EntityValue

	// GetEntitiesByMetricName to get all entities and values by load metric name
	GetEntitiesByMetricName(metricName string) map[*Entity]*EntityValue

	// GetAllMetricNamesWithCount to get all metric names and corresponding entity count
	GetAllMetricNamesWithCount() map[string]int64

	// GetAllNamespaceNamesWithCount to get all namespace names and corresponding entity count
	GetAllNamespaceNamesWithCount() map[string]int64

	// GetEntityByHashKey to get entity and latest value by hash key
	GetEntityByHashKey(hash uint64) (*Entity, *EntityValue)

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
			MetricName: metricName,
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
			}
		}
		if entity.MetricName == "" || entity.Host == "" || entity.EntityType == UnknownType || entity.Namespace == "" || entity.SourceID == "" {
			continue
		}
		for _, point := range points {
			if point != nil && !math.IsNaN(point.Value) {
				entities[&entity] = &EntityValue{
					value:     ValueType(point.Value),
					timestamp: Timestamp(point.Timestamp),
				}
			}
		}
	}
	return entities
}

func ProcessLoadPayload(payload *gogen.MetricPayload, store Store) {
	if payload == nil || store == nil {
		return
	}
	entities := createEntitiesFromPayload(payload)
	store.SetEntitiesValues(entities)
}

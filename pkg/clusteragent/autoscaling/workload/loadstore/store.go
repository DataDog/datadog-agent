// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package loadstore

import (
	"context"
	"strings"
	"sync"

	"github.com/DataDog/agent-payload/v5/gogen"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

var (
	// WorkloadMetricStore is the store for workload metrics
	WorkloadMetricStore Store
	// WorkloadMetricStoreOnce is used to init the store once
	WorkloadMetricStoreOnce sync.Once
)

// GetWorkloadMetricStore returns the workload metric store, init once
func GetWorkloadMetricStore(ctx context.Context) Store {
	WorkloadMetricStoreOnce.Do(func() {
		WorkloadMetricStore = NewEntityStore(ctx)
	})
	return WorkloadMetricStore
}

// StoreInfo represents the store information which aggregates the entities to lowest level, i.e., container level
type StoreInfo struct {
	currentTime  Timestamp
	StatsResults []*StatsResult
}

// StatsResult provides a summary of the entities, grouped by namespace, podOwner, and metric name.
type StatsResult struct {
	Namespace  string
	PodOwner   string
	MetricName string
	Count      int // Under <namespace, podOwner, metric>, number of containers if container type or pods if pod type
}

// PodResult provides the time series of entity values for a pod and its containers
type PodResult struct {
	PodName         string
	ContainerValues map[string][]EntityValue // container name to a time series of entity values, e.g cpu usage from past three collection
	PodLevelValue   []EntityValue            //  If Pod level value is not available, it will be empty
}

// QueryResult provides the pod results for a given query
type QueryResult struct {
	Results []PodResult
}

// Target represents a workload target for filtering (namespace + kind + name)
type Target struct {
	Namespace string
	Kind      string
	Name      string
}

// Store is an interface for in-memory storage of entities and their load metric values.
type Store interface {
	// SetEntitiesValues sets the values for the given map
	SetEntitiesValues(entities map[*Entity]*EntityValue)

	// GetStoreInfo returns the store information.
	GetStoreInfo() StoreInfo

	// GetMetricsRaw provides the values of qualified entities by given search filters
	GetMetricsRaw(metricName string,
		namespace string,
		podOwnerName string,
		containerName string) QueryResult

	// SetTargets sets the list of targets to filter on.
	// Only entities matching one of the targets will be stored.
	// If targets is empty, all entities are accepted (backward compatible - no filtering).
	SetTargets(targets []Target)
}

// createEntitiesFromPayload is a helper function used for creating entities from the metric payload.
func createEntitiesFromPayload(payload *gogen.MetricPayload) map[*Entity]*EntityValue {
	entities := make(map[*Entity]*EntityValue)
	splitTag := func(tag string) (key string, value string) {
		splitIndex := strings.Index(tag, ":")
		if splitIndex < -1 {
			return "", ""
		}
		return tag[:splitIndex], tag[splitIndex+1:]
	}
	for _, series := range payload.Series {
		metricName := series.GetMetric()
		points := series.GetPoints()
		tags := series.GetTags()
		entity := Entity{
			EntityType:   UnknownType,
			EntityName:   "",
			Namespace:    "",
			MetricName:   metricName,
			PodOwnerName: "",
			PodOwnerkind: Unsupported,
		}
		for _, tag := range tags {
			k, v := splitTag(tag)
			switch k {
			case "display_container_name":
				entity.EntityType = ContainerType
				entity.EntityName = v
			case "kube_namespace":
				entity.Namespace = v
			case "container_id":
				entity.EntityType = ContainerType
			case "kube_ownerref_name":
				entity.PodOwnerName = v
			case "kube_ownerref_kind":
				entity.PodOwnerkind = podOwnerTypeFromString(v)
			case "container_name":
				entity.ContainerName = v
			case "pod_name":
				entity.PodName = v
			}
		}
		// TODO:
		// if PodType, populate entity.type first
		// if entity.EntityType == PodType {
		// 		entity.EntityName = entity.PodName
		// }

		// for replicaset, the logic should be consistent with getNamespacedPodOwner in podwatcher
		if entity.PodOwnerkind == ReplicaSet {
			deploymentName := kubernetes.ParseDeploymentForReplicaSet(entity.PodOwnerName)
			if deploymentName != "" {
				entity.PodOwnerkind = Deployment
				entity.PodOwnerName = deploymentName
			} else {
				entity.PodOwnerkind = Unsupported
			}
		}
		if entity.MetricName == "" ||
			entity.EntityType == UnknownType ||
			entity.Namespace == "" ||
			entity.PodOwnerName == "" ||
			entity.EntityName == "" ||
			entity.PodOwnerkind == Unsupported {
			continue
		}
		for _, point := range points {
			if point != nil && point.GetTimestamp() > 0 {
				entities[&entity] = &EntityValue{
					Value:     ValueType(point.GetValue()),
					Timestamp: Timestamp(point.GetTimestamp()),
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
}

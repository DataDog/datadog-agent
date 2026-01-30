// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package loadstore

import (
	"fmt"
	"strings"
	"time"
)

// EntityType defines the type of entity.
type EntityType int8

// PodOwnerType is parsed from kube_ownerref_kind, example values: deployment, statefulset, daemonset, etc.
type PodOwnerType int8

// ValueType defines the datatype of workload value.
type ValueType float64

// Enumeration of entity types.
const (
	ContainerType EntityType = iota
	PodType                  // TODO: PodType is not supported yet
	UnknownType
)

// Enumeration of pod owner types which is parsed from tags kube_ownerref_kind
const (
	Deployment PodOwnerType = iota
	ReplicaSet
	StatefulSet
	Unsupported
)

// podOwnerTypeFromString converts a string kind to PodOwnerType.
// Accepts both capitalized (e.g., "Deployment") and lowercase (e.g., "deployment") formats.
func podOwnerTypeFromString(kind string) PodOwnerType {
	switch strings.ToLower(kind) {
	case "deployment":
		return Deployment
	case "replicaset":
		return ReplicaSet
	case "statefulset":
		return StatefulSet
	default:
		return Unsupported
	}
}

const (
	// maxDataPoints is the maximum number of data points to store per entity.
	maxDataPoints = 3
	// defaultPurgeInterval is the default interval to purge inactive entities.
	defaultPurgeInterval = 3 * time.Minute
	// defaultExpireInterval is the default interval to expire entities.
	defaultExpireInterval = 3 * time.Minute
)

// Entity represents an entity with a type and its attributes.
// if entity is a pod, if entity restarts, a new entity will be created because podname is different
// if entity is a container, the entity will be same
type Entity struct {
	EntityType EntityType // required, PodType or ContainerType

	// Use display_container_name for EntityName if EntityType is container
	// or use podname for entityName if EntityType is pod
	// display_container_name = container.Name + pod.Name
	// if container is restarted, the display_container_name will be the same
	EntityName string // required

	Namespace     string       // required
	PodOwnerName  string       // required, parsed from tags kube_ownerref_name
	PodOwnerkind  PodOwnerType // required, parsed from tags kube_ownerref_kind
	PodName       string       // required, parsed from tags pod_name
	ContainerName string       // optional, short container name, empty if EntityType is PodType
	MetricName    string       // required, metric name of workload
}

// EntityValue represents a value with a timestamp.
type EntityValue struct {
	Value     ValueType
	Timestamp Timestamp
}

// String returns a string representation of the EntityValue.
func (ev *EntityValue) String() string {
	// Convert the timestamp to a time.Time object assuming the timestamp is in seconds.
	// If the timestamp is in milliseconds, use time.UnixMilli(ev.timestamp) instead.
	readableTime := time.Unix(int64(ev.Timestamp), 0).Local().Format(time.RFC3339)
	return fmt.Sprintf("Value: %f, Timestamp: %s", ev.Value, readableTime)
}

// EntityValueQueue represents a queue with a fixed capacity that removes the front element when full
type EntityValueQueue struct {
	data     []*EntityValue
	head     int
	tail     int
	size     int
	capacity int
}

// pushBack adds an element to the back of the queue.
// If the queue is full, it removes the front element first.
func (q *EntityValueQueue) pushBack(value *EntityValue) bool {
	if q.size == q.capacity {
		// Remove the front element
		q.head = (q.head + 1) % q.capacity
		q.size--
	}

	// Add the new element at the back
	q.data[q.tail] = value
	q.tail = (q.tail + 1) % q.capacity
	q.size++
	return true
}

// ToSlice converts the EntityValueQueue data to a slice of EntityValue.
func (q *EntityValueQueue) ToSlice() []EntityValue {
	if q.size == 0 {
		return []EntityValue{}
	}

	result := make([]EntityValue, 0, q.size)
	if q.head < q.tail {
		for _, v := range q.data[q.head:q.tail] {
			result = append(result, *v)
		}
	} else {
		for _, v := range q.data[q.head:] {
			result = append(result, *v)
		}
		for _, v := range q.data[:q.tail] {
			result = append(result, *v)
		}
	}

	return result
}

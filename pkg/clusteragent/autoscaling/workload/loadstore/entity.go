// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package loadstore

import (
	"fmt"
	"time"
	"unsafe"
)

// EntityType defines the type of entity.
type EntityType int

// ValueType defines the datatype of workload value.
type ValueType float64

// Enumeration of entity types.
const (
	ContainerType EntityType = iota
	UnknownType
)

const (
	// maxDataPoints is the maximum number of data points to store per entity.
	maxDataPoints = 3
	// defaultPurgeInterval is the default interval to purge inactive entities.
	defaultPurgeInterval = 3 * time.Minute
	// defaultExpireInterval is the default interval to expire entities.
	defaultExpireInterval = 3 * time.Minute
)

// Entity represents an entity with a type and its attributes.
type Entity struct {
	EntityType EntityType
	SourceID   string
	Host       string // serie.Host
	EntityName string // display_container_name
	Namespace  string
	LoadName   string
	Deployment string
}

// String returns a string representation of the Entity.
func (e *Entity) String() string {
	return fmt.Sprintf(
		"  Key: %d,"+
			"  SourceID: %s,"+
			"  LoadName: %s"+
			"  EntityName: %s,"+
			"  EntityType: %d,"+
			"  Host: %s,"+
			"  Namespace: %s,"+
			"  Deployment: %s",
		hashEntityToUInt64(e), e.SourceID, e.LoadName, e.EntityName, e.EntityType, e.Host, e.Namespace, e.Deployment)
}

// MemoryUsage returns the memory usage of the entity in bytes.
func (e *Entity) MemoryUsage() uint32 {
	return uint32(len(e.SourceID)) + uint32(len(e.Host)) + uint32(len(e.EntityName)) + uint32(len(e.Namespace)) + uint32(len(e.LoadName)) + uint32(unsafe.Sizeof(e.EntityType)) + uint32(len(e.Deployment))
}

// EntityValue represents a value with a timestamp.
type EntityValue struct {
	value     ValueType
	timestamp Timestamp
}

// String returns a string representation of the EntityValue.
func (ev *EntityValue) String() string {
	// Convert the timestamp to a time.Time object assuming the timestamp is in seconds.
	// If the timestamp is in milliseconds, use time.UnixMilli(ev.timestamp) instead.
	readableTime := time.Unix(int64(ev.timestamp), 0).Local().Format(time.RFC3339)
	return fmt.Sprintf("Value: %f, Timestamp: %s", ev.value, readableTime)
}

// EntityValueQueue represents a queue with a fixed capacity that removes the front element when full
type EntityValueQueue struct {
	data     []ValueType
	avg      ValueType
	head     int
	tail     int
	size     int
	capacity int
}

// pushBack adds an element to the back of the queue.
// If the queue is full, it removes the front element first.
func (q *EntityValueQueue) pushBack(value ValueType) bool {
	if q.size == q.capacity {
		// Remove the front element
		q.head = (q.head + 1) % q.capacity
		q.size--
	}

	// Add the new element at the back
	q.data[q.tail] = value
	q.tail = (q.tail + 1) % q.capacity
	q.size++
	q.avg = q.average()
	return true
}

// average calculates the average value of the queue.
func (q *EntityValueQueue) average() ValueType {
	if q.size == 0 {
		return 0
	}
	sum := ValueType(0)
	for i := 0; i < q.size; i++ {
		index := (q.head + i) % q.capacity
		sum += q.data[index]
	}
	return sum / ValueType(q.size)
}

// value returns the average value of the queue
func (q *EntityValueQueue) value() ValueType {
	return q.avg
}

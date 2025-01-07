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

// ValueType defines the datatype of metric value.
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
	MetricName string
}

// String returns a string representation of the Entity.
func (e *Entity) String() string {
	return fmt.Sprintf(
		"  Key: %d,"+
			"  SourceID: %s,"+
			"  MetricName: %s"+
			"  EntityName: %s,"+
			"  EntityType: %d,"+
			"  Host: %s,"+
			"  Namespace: %s",
		hashEntityToUInt64(e), e.SourceID, e.MetricName, e.EntityName, e.EntityType, e.Host, e.Namespace)
}

// MemoryUsage returns the memory usage of the entity in bytes.
func (e *Entity) MemoryUsage() uint32 {
	return uint32(len(e.SourceID)) + uint32(len(e.Host)) + uint32(len(e.EntityName)) + uint32(len(e.Namespace)) + uint32(len(e.MetricName)) + uint32(unsafe.Sizeof(e.EntityType))
}

// EntityValue represents a metric value with a timestamp.
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package loadstore

import (
	"fmt"
	"time"
)

// EntityType defines the type of entity.
type EntityType int

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
	defaultPurgeInterval = 5 * time.Minute
	// defaultExpireInterval is the default interval to expire entities.
	defaultExpireInterval = 5 * time.Minute
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

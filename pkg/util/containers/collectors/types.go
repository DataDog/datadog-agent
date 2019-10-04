// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package collectors

import "github.com/DataDog/datadog-agent/pkg/util/containers"

// Collector is the public interface to container collectors must implement
type Collector interface {
	Detect() error
	List() ([]*containers.Container, error)
	UpdateMetrics([]*containers.Container) error
}

// CollectorPriority helps resolving dupe tags from collectors
type CollectorPriority int

// List of collector priorities
// Same order as the tagger: docker < kubelet
const (
	NodeRuntime CollectorPriority = iota
	NodeOrchestrator
)

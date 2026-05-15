// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hfrunner provides a component that runs system and container checks at
// 1-second intervals and routes their output directly into the observer pipeline.
package hfrunner

// team: q-branch

import (
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const (
	// HFSource is the observer handle source name for HF system check metrics.
	// Using a distinct name from "all-metrics" lets the observer suppress the
	// lower-frequency 15s versions of these metrics when HF mode is active.
	HFSource = "system-checks-hf"

	// HFContainerSource is the observer handle source name for HF container metrics.
	HFContainerSource = "container-checks-hf"
)

// Component is the high-frequency check runner component.
type Component interface {
	// StartSystem starts the HF system check runner using the given handle.
	// systemHandle must be obtained from observer.GetHandle(HFSource).
	// Returns the MetricSource values to suppress from "all-metrics", or nil if
	// the runner was not started (disabled or unavailable on this platform).
	StartSystem(systemHandle observerdef.Handle) map[metrics.MetricSource]struct{}

	// StartContainer starts the HF container check runner using the given handle.
	// containerHandle must be obtained from observer.GetHandle(HFContainerSource).
	// Returns the MetricSource values to suppress from "all-metrics", or nil if
	// not enabled or container deps are unavailable.
	StartContainer(containerHandle observerdef.Handle) map[metrics.MetricSource]struct{}
}

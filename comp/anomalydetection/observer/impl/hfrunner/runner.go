// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hfrunner provides a high-frequency system check runner for the observer.
// This is a stub — the real implementation lands with the algorithm PRs once
// pkg/collector/corechecks/system/* is migrated to Bazel.
package hfrunner

import (
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	taggerdef "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilterdef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetadef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

const (
	// HFSource is the observer handle source name for HF system check metrics.
	HFSource = "system-checks-hf"

	// HFContainerSource is the observer handle source name for HF container metrics.
	HFContainerSource = "container-checks-hf"
)

// Runner runs system checks at 1-second intervals and routes their output
// directly into the observer pipeline.
type Runner struct{}

// ContainerDeps holds the components required to run the generic container check.
type ContainerDeps struct {
	WMeta       workloadmetadef.Component
	FilterStore workloadfilterdef.Component
	Tagger      taggerdef.Component
}

// New creates a no-op Runner. The full implementation is provided once
// pkg/collector/corechecks/system/* is migrated to Bazel.
func New(_ observerdef.Handle) *Runner {
	return &Runner{}
}

// NewContainer creates a no-op Runner for container checks.
func NewContainer(_ observerdef.Handle, _ ContainerDeps) *Runner {
	return &Runner{}
}

// Start is a no-op in the stub.
func (r *Runner) Start() {}

// Stop is a no-op in the stub.
func (r *Runner) Stop() {}

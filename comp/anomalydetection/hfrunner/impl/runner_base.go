// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hfrunnerimpl

import (
	taggerdef "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilterdef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetadef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// systemCheckSources is the set of MetricSource values produced by the system
// checks that the HF runner executes.
var systemCheckSources = map[metrics.MetricSource]struct{}{
	metrics.MetricSourceCPU:        {},
	metrics.MetricSourceLoad:       {},
	metrics.MetricSourceMemory:     {},
	metrics.MetricSourceIo:         {},
	metrics.MetricSourceDisk:       {},
	metrics.MetricSourceNetwork:    {},
	metrics.MetricSourceUptime:     {},
	metrics.MetricSourceFileHandle: {},
}

// containerCheckSources is the set of MetricSource values produced by the
// container checks that the HF container runner executes.
var containerCheckSources = map[metrics.MetricSource]struct{}{
	metrics.MetricSourceContainer: {},
}

// ContainerDeps holds the components required to run the generic container check.
type ContainerDeps struct {
	WMeta       workloadmetadef.Component
	FilterStore workloadfilterdef.Component
	Tagger      taggerdef.Component
}

func copySourceSet(src map[metrics.MetricSource]struct{}) map[metrics.MetricSource]struct{} {
	dst := make(map[metrics.MetricSource]struct{}, len(src))
	for k := range src {
		dst[k] = struct{}{}
	}
	return dst
}

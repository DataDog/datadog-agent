// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package generic

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
)

// SenderFunc is a function that wraps sending metrics
type SenderFunc func(func(string, float64, string, []string), string, *float64, []string)

// ProcessorExtension allows to replace or add optional parts of the core check
type ProcessorExtension interface {
	// PreProcess is called once during check run, before any call to `Process`
	PreProcess(sender SenderFunc, aggSender sender.Sender)

	// Process is called after core process (regardless of encountered error)
	// Tags are given after `AdaptTags()` has been called
	// aggSender is only passed as the sender function (sender.Gauge for instance) needs to be passed back to sender
	Process(tags []string, container *workloadmeta.Container, collector metrics.Collector, cacheValidity time.Duration)

	// PostProcess is called once during each check run, after all calls to `Process`
	PostProcess(tagger tagger.Component)
}

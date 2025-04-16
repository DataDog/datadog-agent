// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package gpusubscriberimpl subscribes to GPU events
package gpusubscriberimpl

import (
	"github.com/DataDog/datadog-agent/pkg/trace/log"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/process/gpusubscriber/def"
	procSubscribers "github.com/DataDog/datadog-agent/pkg/process/subscribers"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

type gpusubscriberimpl struct {
	gpuSubscriber *procSubscribers.GPUSubscriber
}

// Requires defines the dependencies of the gpu subscriber component.
type Requires struct {
	compdef.In
	Lc compdef.Lifecycle

	WMeta  workloadmeta.Component
	Tagger tagger.Component
}

// NewComponent returns a new gpu subscriber.
func NewComponent(reqs Requires) gpusubscriber.Component {
	if flavor.GetFlavor() == flavor.ProcessAgent {
		log.Debug("GPU subscriber disabled as running in Process Agent")
		return NoopSubscriber{}
	}

	gpuSubscriber := procSubscribers.NewGPUSubscriber(reqs.WMeta, reqs.Tagger)
	gpuSubComponent := gpusubscriberimpl{
		gpuSubscriber: gpuSubscriber,
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: gpuSubscriber.Run,
		OnStop:  gpuSubscriber.Stop,
	})
	return gpuSubComponent
}

func (g gpusubscriberimpl) GetGPUTags() map[int32][]string {
	if g.gpuSubscriber == nil {
		return map[int32][]string{}
	}
	return g.gpuSubscriber.GetGPUTags()
}

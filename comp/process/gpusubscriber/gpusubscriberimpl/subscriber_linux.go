// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package gpusubscriberimpl implements a component to handle GPU detection in the Core Agent.
package gpusubscriberimpl

import (
	"go.uber.org/fx"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/process/gpusubscriber"
	procSubscribers "github.com/DataDog/datadog-agent/pkg/process/subscribers"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newGpuSubscriber))
}

type gpusubscriberimpl struct {
	gpuSubscriber *procSubscribers.GPUSubscriber
}

type dependencies struct {
	fx.In
	Lc fx.Lifecycle

	WMeta  workloadmeta.Component
	Tagger tagger.Component
}

func newGpuSubscriber(deps dependencies) gpusubscriber.Component {
	gpuSubscriber := procSubscribers.NewGPUSubscriber(deps.WMeta, deps.Tagger)
	gpuSubComponent := gpusubscriberimpl{
		gpuSubscriber: gpuSubscriber,
	}

	// TODO: only run in core agent (not process agent), when gpu & process check is enabled?
	if flavor.GetFlavor() == flavor.ProcessAgent {
		// TODO: put a debug statement here to indicate we're in the process-agent and this is not enabled
		return NoopSubscriber{}
	}

	deps.Lc.Append(fx.Hook{
		OnStart: gpuSubscriber.Run,
		OnStop:  gpuSubscriber.Stop,
	})
	return gpuSubComponent
}

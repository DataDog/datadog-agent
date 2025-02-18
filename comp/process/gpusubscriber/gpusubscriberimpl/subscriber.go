// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gpusubscriberimpl implements a component to handle GPU detection in the Core Agent.
package gpusubscriberimpl

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/process/gpusubscriber"
	procSubscribers "github.com/DataDog/datadog-agent/pkg/process/subscribers"
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

	Sysconfig sysprobeconfig.Component
	Config    config.Component
	WMeta     workloadmeta.Component
	Tagger    tagger.Component
}

type result struct {
	fx.Out

	Component gpusubscriber.Component
}

func newGpuSubscriber(deps dependencies) result {
	gpuSubscriber := procSubscribers.NewGPUSubscriber(deps.WMeta)
	return result{
		Component: gpuSubscriber,
	}
}

func (g *gpusubscriberimpl) Run(context.Context) error {
	go g.gpuSubscriber.Run()
	return nil
}

func (g *gpusubscriberimpl) Stop(context.Context) error {
	g.gpuSubscriber.Stop()
	return nil
}

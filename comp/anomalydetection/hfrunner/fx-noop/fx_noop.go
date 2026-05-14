// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the hfrunner component.
// This no-op variant provides a stub that never starts any runner.
// Wire hfrunner/fx instead when high-frequency check collection is needed.
package fx

import (
	hfrunnerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/hfrunner/def"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the no-op hfrunner component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(newNoopHFRunner),
	)
}

type noopRequires struct{}
type noopProvides struct {
	Comp hfrunnerdef.Component
}

func newNoopHFRunner(_ noopRequires) noopProvides {
	return noopProvides{Comp: &noopHFRunner{}}
}

type noopHFRunner struct{}

func (n *noopHFRunner) StartSystem(_ observerdef.Handle) map[metrics.MetricSource]struct{} {
	return nil
}

func (n *noopHFRunner) StartContainer(_ observerdef.Handle) map[metrics.MetricSource]struct{} {
	return nil
}

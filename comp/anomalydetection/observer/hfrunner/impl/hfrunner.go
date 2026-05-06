// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hfrunnerimpl

import (
	"context"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	hfrunnerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/hfrunner/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	taggerdef "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilterdef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetadef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires declares the input types to the hfrunner component constructor.
type Requires struct {
	compdef.In

	Lifecycle   compdef.Lifecycle
	Config      config.Component
	WMeta       option.Option[workloadmetadef.Component]
	FilterStore option.Option[workloadfilterdef.Component]
	Tagger      option.Option[taggerdef.Component]
}

// Provides defines the output of the hfrunner component.
type Provides struct {
	compdef.Out

	Comp hfrunnerdef.Component
}

type hfrunnerComp struct {
	lifecycle        compdef.Lifecycle
	systemEnabled    bool
	containerEnabled bool
	wmeta            option.Option[workloadmetadef.Component]
	filterStore      option.Option[workloadfilterdef.Component]
	tagger           option.Option[taggerdef.Component]
}

// NewComponent creates the hfrunner component.
func NewComponent(deps Requires) Provides {
	return Provides{
		Comp: &hfrunnerComp{
			lifecycle:        deps.Lifecycle,
			systemEnabled:    deps.Config.GetBool("observer.high_frequency_system_checks.enabled"),
			containerEnabled: deps.Config.GetBool("observer.high_frequency_container_checks.enabled"),
			wmeta:            deps.WMeta,
			filterStore:      deps.FilterStore,
			tagger:           deps.Tagger,
		},
	}
}

// StartSystem starts the HF system check runner with the given handle.
// Returns the MetricSource values to suppress from "all-metrics", or nil if
// the runner was not started.
func (h *hfrunnerComp) StartSystem(systemHandle observerdef.Handle) map[metrics.MetricSource]struct{} {
	if !h.systemEnabled {
		return nil
	}
	r := newRunner(systemHandle)
	r.start()
	pkglog.Info("[observer] high-frequency system check runner started (1s interval)")
	h.lifecycle.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			r.stop()
			return nil
		},
	})
	return copySourceSet(systemCheckSources)
}

// StartContainer starts the HF container check runner with the given handle.
// Returns MetricSource values to suppress, or nil if not enabled or deps unavailable.
func (h *hfrunnerComp) StartContainer(containerHandle observerdef.Handle) map[metrics.MetricSource]struct{} {
	if !h.containerEnabled {
		return nil
	}
	wmeta, wok := h.wmeta.Get()
	filterStore, fok := h.filterStore.Get()
	tagger, tok := h.tagger.Get()
	if !wok || !fok || !tok {
		pkglog.Warn("[observer] high_frequency_container_checks.enabled=true but WMeta/FilterStore/Tagger not available; skipping")
		return nil
	}
	r := newContainerRunner(containerHandle, ContainerDeps{
		WMeta:       wmeta,
		FilterStore: filterStore,
		Tagger:      tagger,
	})
	r.start()
	pkglog.Info("[observer] high-frequency container check runner started (1s interval)")
	h.lifecycle.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			r.stop()
			return nil
		},
	})
	return copySourceSet(containerCheckSources)
}

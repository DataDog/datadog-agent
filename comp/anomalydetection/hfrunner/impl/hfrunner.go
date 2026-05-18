// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hfrunnerimpl implements the hfrunner component. It runs system and
// container checks at 1-second intervals and routes their output directly into
// the observer pipeline, bypassing the normal aggregator/forwarder chain.
package hfrunnerimpl

import (
	"context"

	hfrunnerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/hfrunner/def"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
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
	systemEnabled    bool
	containerEnabled bool
	wmeta            option.Option[workloadmetadef.Component]
	filterStore      option.Option[workloadfilterdef.Component]
	tagger           option.Option[taggerdef.Component]
	stoppers         []func()
}

// NewComponent creates the hfrunner component.
func NewComponent(deps Requires) Provides {
	h := &hfrunnerComp{
		systemEnabled:    deps.Config.GetBool("anomaly_detection.checks.high_frequency_system"),
		containerEnabled: deps.Config.GetBool("anomaly_detection.checks.high_frequency_containers"),
		wmeta:            deps.WMeta,
		filterStore:      deps.FilterStore,
		tagger:           deps.Tagger,
	}
	deps.Lifecycle.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			for _, stop := range h.stoppers {
				stop()
			}
			return nil
		},
	})
	return Provides{Comp: h}
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
	pkglog.Info("[observer/hfrunner] high-frequency system check runner started (1s interval)")
	h.stoppers = append(h.stoppers, r.stop)
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
		pkglog.Warn("[observer/hfrunner] anomaly_detection.checks.high_frequency_containers=true but WMeta/FilterStore/Tagger not available; skipping")
		return nil
	}
	r := newContainerRunner(containerHandle, ContainerDeps{
		WMeta:       wmeta,
		FilterStore: filterStore,
		Tagger:      tagger,
	})
	if r == nil {
		return nil
	}
	r.start()
	pkglog.Info("[observer/hfrunner] high-frequency container check runner started (1s interval)")
	h.stoppers = append(h.stoppers, r.stop)
	return copySourceSet(containerCheckSources)
}

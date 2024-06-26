// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package workloadmeta

// team: container-platform

import (
	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	wmmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
)

const (
	mockSource = wmdef.Source("mockSource")
)

// MockStore is a store designed to be used in unit tests
type workloadMetaMockV2 struct {
	*workloadmeta
}

// NewWorkloadMetaMockV2 returns a Mock
func NewWorkloadMetaMockV2(deps dependencies) wmmock.Mock {
	w := &workloadMetaMockV2{
		workloadmeta: NewWorkloadMeta(deps).Comp.(*workloadmeta),
	}
	return w
}

// Notify overrides store to allow for synchronous event processing
func (w *workloadMetaMockV2) Notify(events []wmdef.CollectorEvent) {
	w.handleEvents(events)
}

// SetEntity generates a Set event
func (w *workloadMetaMockV2) Set(e wmdef.Entity) {
	w.Notify([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: mockSource,
			Entity: e,
		},
	})
}

// GetConfig returns a Config Reader for the internal injected config
func (w *workloadMetaMockV2) GetConfig() pkgconfig.Reader {
	return w.config
}

// UnsetEntity generates an Unset event
func (w *workloadMetaMockV2) Unset(e wmdef.Entity) {
	w.Notify([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeUnset,
			Source: mockSource,
			Entity: e,
		},
	})
}

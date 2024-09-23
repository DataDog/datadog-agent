// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package workloadmetaimpl

// team: container-platform

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	wmmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
)

const (
	mockSource = wmdef.Source("mockSource")
)

// MockStore is a store designed to be used in unit tests
type workloadMetaMock struct {
	*workloadmeta
}

// NewWorkloadMetaMock returns a Mock
func NewWorkloadMetaMock(deps Dependencies) wmmock.Mock {
	w := &workloadMetaMock{
		workloadmeta: NewWorkloadMeta(deps).Comp.(*workloadmeta),
	}
	return w
}

// Notify overrides store to allow for synchronous event processing
func (w *workloadMetaMock) Notify(events []wmdef.CollectorEvent) {
	w.handleEvents(events)
}

// Set generates a Set event
func (w *workloadMetaMock) Set(e wmdef.Entity) {
	w.Notify([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: mockSource,
			Entity: e,
		},
	})
}

// GetConfig returns a Config Reader for the internal injected config
func (w *workloadMetaMock) GetConfig() config.Reader {
	return w.config
}

// Unset generates an Unset event
func (w *workloadMetaMock) Unset(e wmdef.Entity) {
	w.Notify([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeUnset,
			Source: mockSource,
			Entity: e,
		},
	})
}

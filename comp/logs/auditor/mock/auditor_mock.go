// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock is the mock component for the auditor
package mock

import (
	compdef "github.com/DataDog/datadog-agent/comp/def"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// ProvidesMock is the mock component output
type ProvidesMock struct {
	compdef.Out

	Comp auditor.Component
}

// AuditorMockModule defines the fx options for the mock component.
func AuditorMockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(newMock),
	)
}

func newMock() ProvidesMock {
	return ProvidesMock{
		Comp: NewMockAuditor(),
	}
}

// NewMockAuditor returns a new mock auditor
func NewMockAuditor() *Auditor {
	return &Auditor{
		Registry:         Registry{},
		channel:          make(chan *message.Payload),
		stopChannel:      make(chan struct{}),
		ReceivedMessages: make([]*message.Payload, 0),
	}
}

// Auditor is a mock auditor that does nothing
type Auditor struct {
	Registry
	channel          chan *message.Payload
	stopChannel      chan struct{}
	ReceivedMessages []*message.Payload
}

// Start starts the NullAuditor main loop
func (a *Auditor) Start() {
	go a.run()
}

// Stop stops the NullAuditor main loop
func (a *Auditor) Stop() {
	a.stopChannel <- struct{}{}
}

// Channel returns the channel messages should be sent on
func (a *Auditor) Channel() chan *message.Payload {
	return a.channel
}

// run is the main run loop for the null auditor
func (a *Auditor) run() {
	for {
		select {
		case val := <-a.channel:
			a.ReceivedMessages = append(a.ReceivedMessages, val)
		case <-a.stopChannel:
			close(a.channel)
			return
		}
	}
}

// Registry does nothing
type Registry struct {
	offset      string
	tailingMode string
}

// NewMockRegistry returns a new mock registry.
func NewMockRegistry() *Registry {
	return &Registry{}
}

// GetOffset returns the offset.
func (r *Registry) GetOffset(_ string) string {
	return r.offset
}

// SetOffset sets the offset.
func (r *Registry) SetOffset(offset string) {
	r.offset = offset
}

// GetTailingMode returns the tailing mode.
func (r *Registry) GetTailingMode(_ string) string {
	return r.tailingMode
}

// SetTailingMode sets the tailing mode.
func (r *Registry) SetTailingMode(tailingMode string) {
	r.tailingMode = tailingMode
}

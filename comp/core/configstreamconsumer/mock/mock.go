// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides mock for configstreamconsumer component
package mock

import (
	"context"
	"testing"

	configstreamconsumer "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// Mock is a mock implementation of configstreamconsumer.Component
type Mock struct {
	t      *testing.T
	reader model.Reader
	ready  bool
}

// New creates a new mock configstreamconsumer component
func New(t *testing.T) configstreamconsumer.Component {
	return &Mock{
		t:     t,
		ready: true,
	}
}

// NewWithReader creates a new mock with a custom reader
func NewWithReader(t *testing.T, reader model.Reader) configstreamconsumer.Component {
	return &Mock{
		t:      t,
		reader: reader,
		ready:  true,
	}
}

// Start implements configstreamconsumer.Component
func (m *Mock) Start(_ context.Context) error {
	return nil
}

// WaitReady implements configstreamconsumer.Component
func (m *Mock) WaitReady(_ context.Context) error {
	if !m.ready {
		return context.DeadlineExceeded
	}
	return nil
}

// Reader implements configstreamconsumer.Component
func (m *Mock) Reader() model.Reader {
	return m.reader
}

// Subscribe implements configstreamconsumer.Component
func (m *Mock) Subscribe() (<-chan configstreamconsumer.ChangeEvent, func()) {
	ch := make(chan configstreamconsumer.ChangeEvent)
	return ch, func() { close(ch) }
}

// SetReady sets the ready state for testing
func (m *Mock) SetReady(ready bool) {
	m.ready = ready
}

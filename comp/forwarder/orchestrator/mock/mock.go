// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package orchestratormock provides a mock for the orchestrator forwarder component.
package orchestratormock

import (
	"testing"

	defaultforwarderdef "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwardernoop "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/noop-impl"
	orchestrator "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Mock implements the orchestrator forwarder component.
type Mock struct {
	forwarder option.Option[defaultforwarderdef.Forwarder]
}

// New returns a mock orchestrator forwarder.
func New(_ testing.TB) orchestrator.Component {
	forwarder := option.New[defaultforwarderdef.Forwarder](defaultforwardernoop.NewComponent())
	return &Mock{forwarder: forwarder}
}

// Get returns the mock forwarder.
func (m *Mock) Get() (defaultforwarderdef.Forwarder, bool) {
	return m.forwarder.Get()
}

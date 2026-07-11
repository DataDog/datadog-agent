// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package orchestratormock provides a mock for the orchestrator forwarder component.
package orchestratormock

import (
	defaultforwarderdef "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwardernoop "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/noop-impl"
	orchestrator "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// MockModule defines the fx options for this mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(NewMockOrchestratorForwarder))
}

// NewMockOrchestratorForwarder returns a mock orchestratorForwarder.
func NewMockOrchestratorForwarder() orchestrator.Component {
	forwarder := option.New[defaultforwarderdef.Forwarder](defaultforwardernoop.NewComponent())
	return &forwarder
}

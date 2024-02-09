// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package orchestratorimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// MockModule defines the fx options for this mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMockOrchestratorForwarder))
}

// NewMockOrchestratorForwarder returns an orchestratorForwarder
func NewMockOrchestratorForwarder() orchestrator.Component {
	forwarder := optional.NewOption[defaultforwarder.Forwarder](defaultforwarder.NoopForwarder{})
	return &forwarder
}

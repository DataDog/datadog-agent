// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package forwarderimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/orchestrator/forwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for this mock component.
var MockModule = fxutil.Component(
	fx.Provide(NewMockOrchestratorForwarder),
)

// NewMockOrchestratorForwarder returns an orchestratorForwarder
func NewMockOrchestratorForwarder() forwarder.Component {
	return defaultforwarder.NoopForwarder{}

}

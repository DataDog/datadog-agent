// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !orchestrator

package forwarderimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/orchestrator/forwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(NewOrchestratorForwarder),
)

// NewOrchestratorForwarder builds the orchestrator forwarder.
// This func has been extracted in this file to not include all the orchestrator
// dependencies (k8s, several MBs) while building binaries not needing these.
func NewOrchestratorForwarder(log log.Component, config config.Component, params Params) forwarder.Component {
	// do not return any forwarder for builds without the orchestrator tag
	return nil // TODO: (Components): Use optional instead
}

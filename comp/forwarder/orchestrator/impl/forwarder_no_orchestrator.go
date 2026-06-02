// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !orchestrator

// Package orchestratorimpl implements the orchestrator forwarder component.
package orchestratorimpl

import (
	"go.uber.org/fx"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	orchestrator "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// noOrchRequires holds the dependencies for the no-orchestrator forwarder build.
type noOrchRequires struct {
	compdef.In

	Params orchestrator.Params
}

// Module defines the fx options for this component.
func Module(params orchestrator.Params) fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(newOrchestratorForwarder),
		fx.Supply(params),
	)
}

// newOrchestratorForwarder builds the orchestrator forwarder.
// This func has been extracted in this file to not include all the orchestrator
// dependencies (k8s, several MBs) while building binaries not needing these.
func newOrchestratorForwarder(deps noOrchRequires) orchestrator.Component {
	if deps.Params.UseNoopOrchestratorForwarder() {
		forwarder := option.New[defaultforwarder.Forwarder](defaultforwarder.NoopForwarder{})
		return &forwarder
	}
	forwarder := option.None[defaultforwarder.Forwarder]()
	return &forwarder
}

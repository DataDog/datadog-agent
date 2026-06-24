// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package helmactionsimpl implements the helmactions component interface.
package helmactionsimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	helmactions "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/def"
)

// Requires defines the dependencies for the helmactions component.
type Requires struct {
	Lifecycle compdef.Lifecycle

	Log    log.Component
	Config config.Component
	Params helmactions.Params
}

// Provides defines the output of the helmactions component.
type Provides struct {
	Comp helmactions.Component
}

type helmactionsImpl struct {
	log    log.Component
	config config.Component
	params helmactions.Params
}

// NewComponent creates a new helmactions component.
func NewComponent(reqs Requires) (Provides, error) {
	comp := &helmactionsImpl{
		log:    reqs.Log,
		config: reqs.Config,
		params: reqs.Params,
	}

	reqs.Lifecycle.Append(compdef.Hook{OnStart: comp.start, OnStop: comp.stop})

	return Provides{Comp: comp}, nil
}

func (h *helmactionsImpl) start(_ context.Context) error {
	h.log.Info("Starting helmactions component")
	return nil
}

func (h *helmactionsImpl) stop(_ context.Context) error {
	h.log.Info("Stopping helmactions component")
	return nil
}

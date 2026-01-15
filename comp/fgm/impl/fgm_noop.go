// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package fgmimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	fgmdef "github.com/DataDog/datadog-agent/comp/fgm/def"
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires defines the dependencies for the FGM component (noop version)
type Requires struct {
	fx.In

	Lc           fx.Lifecycle
	Log          log.Component
	Config       config.Component
	Workloadmeta workloadmeta.Component
	Observer     option.Option[observer.Component]
}

type noopComponent struct{}

// NewComponent returns a no-op implementation for non-Linux platforms
func NewComponent(reqs Requires) (fgmdef.Component, error) {
	reqs.Log.Info("fgm: Component not available on non-Linux platforms")
	return &noopComponent{}, nil
}

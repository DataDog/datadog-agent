// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the runner component.
package fx

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	hostinfo "github.com/DataDog/datadog-agent/comp/process/hostinfo/def"
	runner "github.com/DataDog/datadog-agent/comp/process/runner/def"
	runnerimpl "github.com/DataDog/datadog-agent/comp/process/runner/impl"
	submitter "github.com/DataDog/datadog-agent/comp/process/submitter/def"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// fxDependencies holds the fx-injected dependencies for the runner, including
// the check group. Filtering of nil/typed-nil values happens here so that
// impl.NewComponent remains free of fxutil.
type fxDependencies struct {
	compdef.In

	Lc  compdef.Lifecycle
	Log log.Component

	Submitter  submitter.Component
	RTNotifier <-chan types.RTResponse `optional:"true"`

	Checks   []types.CheckComponent `group:"check"`
	HostInfo hostinfo.Component
	SysCfg   sysprobeconfig.Component
	Config   config.Component
	Tagger   tagger.Component
}

func newComponent(deps fxDependencies) (runner.Component, error) {
	return runnerimpl.NewComponent(runnerimpl.Requires{
		Lc:         deps.Lc,
		Log:        deps.Log,
		Submitter:  deps.Submitter,
		RTNotifier: deps.RTNotifier,
		// Filter nil and typed-nil values from the fx group before passing to impl.
		Checks:   fxutil.GetAndFilterGroup(deps.Checks),
		HostInfo: deps.HostInfo,
		SysCfg:   deps.SysCfg,
		Config:   deps.Config,
		Tagger:   deps.Tagger,
	})
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newComponent),
	)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package autoexitimpl implements autoexit.Component
package autoexitimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/manager"
	"github.com/DataDog/datadog-agent/comp/agent/autoexit"
	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgcommon "github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newAutoExit),
	)
}

type dependencies struct {
	fx.In

	Config config.Component
}

// ExitDetector is common interface for shutdown mechanisms
type ExitDetector interface {
	check() bool
}

// ConfigureAutoExit starts automatic shutdown mechanism if necessary
func newAutoExit(deps dependencies) (autoexit.Component, error) {

	ctx, _ := pkgcommon.GetMainCtxCancel()
	err := manager.ConfigureAutoExit(ctx, deps.Config)

	return struct{}{}, err
}

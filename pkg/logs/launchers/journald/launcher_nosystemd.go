// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !systemd

//nolint:revive // TODO(AML) Fix revive linter
package journald

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	flareController "github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
)

// Launcher is not supported on no systemd environment.
type Launcher struct{}

// NewLauncher returns a new Launcher
func NewLauncher(*flareController.FlareController, tagger.Component) *Launcher {
	return &Launcher{}
}

// Start does nothing
//
//nolint:revive // TODO(AML) Fix revive linter
func (l *Launcher) Start(_ launchers.SourceProvider, _ pipeline.Provider, _ auditor.Registry, _ *tailers.TailerTracker) {
}

// Stop does nothing
func (l *Launcher) Stop() {}

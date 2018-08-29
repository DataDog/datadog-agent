// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !docker

package docker

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// Launcher is not supported on windows environment
type Launcher struct{}

// NewLauncher returns a new Launcher
func NewLauncher(sources *config.LogSources, pipelineProvider pipeline.Provider, registry auditor.Registry) (*Launcher, error) {
	return &Launcher{}, nil
}

// Start does nothing
func (l *Launcher) Start() {}

// Stop does nothing
func (l *Launcher) Stop() {}

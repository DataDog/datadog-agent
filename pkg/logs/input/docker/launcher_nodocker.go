// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !docker

package docker

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

// Launcher is not supported on non docker environment
type Launcher struct{}

// NewLauncher returns a new Launcher
func NewLauncher(readTimeout time.Duration, psources *config.LogSources, services *service.Services, pipelineProvider pipeline.Provider, registry auditor.Registry, shouldRetry bool) (*Launcher, error) {
	return &Launcher{}, nil
}

// Start does nothing
func (l *Launcher) Start() {}

// Stop does nothing
func (l *Launcher) Stop() {}

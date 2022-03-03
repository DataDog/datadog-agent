// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet
// +build !kubelet

package kubernetes

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// Launcher is not supported on no kubelet environment
type Launcher struct{}

// NewLauncher returns a new launcher
func NewLauncher(sources *config.LogSources, services *service.Services, collectAll bool) *Launcher {
	return &Launcher{}
}

// IsAvailable retrurns false - not available
func IsAvailable() (bool, *retry.Retrier) {
	return false, nil
}

// Start does nothing
func (l *Launcher) Start(sourceProider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry) {
}

// Stop does nothing
func (l *Launcher) Stop() {}

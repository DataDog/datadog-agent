// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !kubelet

package kubernetes

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

// Launcher is not supported on no kubelet environment
type Launcher struct{}

// NewLauncher returns a new launcher
func NewLauncher(sources *config.LogSources, services *service.Services, collectAll bool) (*Launcher, error) {
	return &Launcher{}, nil
}

// Start does nothing
func (l *Launcher) Start() {}

// Stop does nothing
func (l *Launcher) Stop() {}

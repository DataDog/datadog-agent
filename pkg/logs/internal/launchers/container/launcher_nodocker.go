// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !docker
// +build !docker

package container

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	sourcesPkg "github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// A Launcher starts and stops new tailers for every new containers discovered by autodiscovery.
//
// Due to lack of the `docker` build tag, this type is a dummy and does nothing.
type Launcher struct{}

// NewLauncher returns a new launcher
func NewLauncher(sources *sourcesPkg.LogSources) *Launcher {
	return &Launcher{}
}

// Start implements Launcher#Start.
func (l *Launcher) Start(launchers.SourceProvider, pipeline.Provider, auditor.Registry) {
}

// Stop implements Launcher#Stop.
func (l *Launcher) Stop() {
}

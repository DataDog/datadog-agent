// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet && !docker

// Package container provides container-based log launchers
package container

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	sourcesPkg "github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// A Launcher starts and stops new tailers for every new containers discovered by autodiscovery.
//
// Due to lack of the `docker` build tag, this type is a dummy and does nothing.
type Launcher struct{}

// NewLauncher returns a new launcher
func NewLauncher(_ *sourcesPkg.LogSources, _ option.Option[workloadmeta.Component], _ tagger.Component) *Launcher {
	return &Launcher{}
}

// Start implements Launcher#Start.
func (l *Launcher) Start(launchers.SourceProvider, pipeline.Provider, auditor.Registry, *tailers.TailerTracker) {
}

// Stop implements Launcher#Stop.
func (l *Launcher) Stop() {
}

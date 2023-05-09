// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package windowsevent

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

type Launcher struct{}

func (l *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry) {
	log.Warn("windows event log not supported on this system")
}

func (t *Launcher) Stop() {}

func NewLauncher() *Launcher {
	return &Launcher{}
}

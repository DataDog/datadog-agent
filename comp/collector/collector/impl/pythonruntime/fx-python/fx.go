// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build python

// Package fxpython provides the Python-backed collector runtime initializer.
package fxpython

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/collector/collector/impl/pythonruntime"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	collectorpython "github.com/DataDog/datadog-agent/pkg/collector/python"
)

type runtime struct{}

func (runtime) SetHealthPlatform(h healthplatform.Component) {
	collectorpython.SetHealthPlatform(h)
}

func (runtime) InitPython(paths ...string) {
	collectorpython.InitPython(paths...)
}

func (runtime) TerminateRunningProcesses() {
	collectorpython.TerminateRunningProcesses()
}

// Module provides Python runtime capabilities for the collector.
func Module() fx.Option {
	return fx.Module(
		"comp/collector/collector/impl/pythonruntime/fx-python",
		fx.Provide(func() pythonruntime.Runtime {
			return runtime{}
		}),
	)
}

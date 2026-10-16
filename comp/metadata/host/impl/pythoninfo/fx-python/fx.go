// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fxpython provides Python-backed host metadata version information.
package fxpython

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/metadata/host/impl/pythoninfo"
	collectorpython "github.com/DataDog/datadog-agent/pkg/collector/python"
)

type provider struct{}

func (provider) GetPythonInfo() string {
	return collectorpython.GetPythonInfo()
}

func (provider) GetPythonVersion() string {
	return collectorpython.GetPythonVersion()
}

// Module provides a Python version information provider for host metadata.
func Module() fx.Option {
	return fx.Module(
		"comp/metadata/host/impl/pythoninfo/fx-python",
		fx.Provide(func() pythoninfo.Provider {
			return provider{}
		}),
	)
}

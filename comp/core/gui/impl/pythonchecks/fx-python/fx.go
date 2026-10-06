// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fxpython provides a Python-backed GUI integration lister.
package fxpython

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/gui/impl/pythonchecks"
	collectorpython "github.com/DataDog/datadog-agent/pkg/collector/python"
)

type lister struct{}

func (lister) GetPythonIntegrationList() ([]string, error) {
	return collectorpython.GetPythonIntegrationList()
}

// Module provides a Python integration lister for the GUI.
func Module() fx.Option {
	return fx.Module(
		"comp/core/gui/impl/pythonchecks/fx-python",
		fx.Provide(func() pythonchecks.Lister {
			return lister{}
		}),
	)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package containercheck implements mock for the containercheck component.
package containercheck

import (
	"go.uber.org/fx"

	containercheckimpl "github.com/DataDog/datadog-agent/comp/process/containercheck/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(containercheckimpl.NewMock),
	)
}

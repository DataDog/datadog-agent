// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package submitter implements a component to submit collected data in the Process Agent to
// supported Datadog intakes.
package submitter

import (
	"go.uber.org/fx"

	submitter "github.com/DataDog/datadog-agent/comp/process/submitter/def"
	submitterimpl "github.com/DataDog/datadog-agent/comp/process/submitter/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Mock implements mock-specific methods.
type Mock interface {
	submitter.Component
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(submitterimpl.NewMock),
	)
}

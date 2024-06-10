// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package fx

import (
	"go.uber.org/fx"

	traceagentdef "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	traceagentimpl "github.com/DataDog/datadog-agent/comp/trace/agent/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Mock implements mock-specific methods.
type Mock interface {
	traceagentdef.Component
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(traceagentimpl.NewMock),
	)
}

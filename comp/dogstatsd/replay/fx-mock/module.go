// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package server implements a component to run the dogstatsd capture/replay
//
//nolint:revive // TODO(AML) Fix revive linter
package fxmock

import (
	replaymock "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			replaymock.NewTrafficCapture,
		),
	)
}

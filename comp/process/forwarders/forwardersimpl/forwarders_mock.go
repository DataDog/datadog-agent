// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package forwardersimpl implements a component to provide forwarders used by the process agent.
package forwardersimpl

import (
	"github.com/DataDog/datadog-agent/comp/process/forwarders"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// MockModule is the mock module for process forwarders
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMockForwarders),
	)
}

func newMockForwarders(deps dependencies) (forwarders.Component, error) {
	return newForwarders(deps)
}

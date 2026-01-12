// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package forwardersimpl implements a component to provide forwarders used by the process agent.
package forwardersimpl

import (
	"testing"

	"go.uber.org/fx"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/comp/process/forwarders"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule is the mock module for process forwarders
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMockForwarders),
		//TODO: Fix the MockForwarder to be a real mock,
		// and remove the need of including the MockSecrets for tests that use only the Forwarder.
		fx.Provide(func(t testing.TB) secrets.Component { return secretsmock.New(t) }),
	)
}

func newMockForwarders(deps dependencies) (forwarders.Component, error) {
	return newForwarders(deps)
}

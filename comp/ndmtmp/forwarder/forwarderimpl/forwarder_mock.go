// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package forwarderimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/golang/mock/gomock"
	"go.uber.org/fx"
)

func getMockForwarder(t testing.TB) forwarder.MockComponent {
	ctrl := gomock.NewController(t)
	return eventplatformimpl.NewMockEventPlatformForwarder(ctrl)
}

// MockModule defines a component with a mock forwarder
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(
			getMockForwarder,
			// Provide the mock as the primary component as well
			func(c forwarder.MockComponent) forwarder.Component { return c },
		))
}

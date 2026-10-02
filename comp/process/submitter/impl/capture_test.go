// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package submitterimpl

import (
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
	connectionsforwardermock "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/mock"
	forwardersimpl "github.com/DataDog/datadog-agent/comp/process/forwarders/mock"
	hostinfomock "github.com/DataDog/datadog-agent/comp/process/hostinfo/mock"
	submitter "github.com/DataDog/datadog-agent/comp/process/submitter/def"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestConstructedProcessComponentSharesManagerWithoutReadiness(t *testing.T) {
	manager := telemetrycapture.NewManager("core-agent", "fixture", "fixture")
	defer manager.Close()
	s := fxutil.Test[submitter.Component](t, fx.Options(
		hostinfomock.MockModule(), core.MockBundle(),
		fx.Provide(func() connectionsforwarder.Component { return connectionsforwardermock.Mock(t) }),
		forwardersimpl.MockModule(),
		fx.Provide(func() statsd.ClientInterface { return &statsd.NoOpClient{} }),
		fx.Supply(manager), fxutil.ProvideComponentConstructor(NewComponent),
	))
	require.Same(t, manager, s.(*submitterImpl).s.CaptureManager)
	require.Empty(t, manager.Status().Capabilities)
}

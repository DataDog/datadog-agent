// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package fx

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	dogstatsdclientdropdetector "github.com/DataDog/datadog-agent/comp/aggregator/dogstatsdclientdropdetector/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In

	Detector dogstatsdclientdropdetector.Component
}

func TestModuleProvidesDetectorAndFlushListener(t *testing.T) {
	deps := fxutil.Test[dependencies](t,
		Module(),
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() hostnameinterface.Component {
			hostname, _ := hostnamemock.NewMock(hostnamemock.MockHostname("test-node"))
			return hostname
		}),
		fx.Provide(func() healthplatformstore.Component { return healthplatformmock.New(t) }),
	)

	require.NotNil(t, deps.Detector)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package snmptraps

import (
	"testing"

	demultiplexerimpl "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/impl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	defaultforwardermock "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle(),
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		hostnameimpl.MockModule(),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		demultiplexerimpl.MockModule(),
		defaultforwardermock.MockModule(),
	)
}

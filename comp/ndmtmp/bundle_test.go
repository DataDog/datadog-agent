// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package ndmtmp

import (
	"testing"

	demultiplexerimpl "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/impl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	eventplatformmock "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/mock"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	ddagg "github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle(),
		demultiplexerimpl.MockModule(),
		orchestratorForwarderImpl.MockModule(),
		defaultforwarder.MockModule(),
		eventplatformmock.MockModule(),
		core.MockBundle(),
		hostnameimpl.MockModule(),
	)
}

func TestMockBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, MockBundle(),
		core.MockBundle(),
		fx.Provide(func() *ddagg.AgentDemultiplexer {
			return &ddagg.AgentDemultiplexer{}
		}),
	)
}

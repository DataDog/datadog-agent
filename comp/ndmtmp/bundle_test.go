// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package ndmtmp

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	ddagg "github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle,
		demultiplexer.Module,
		defaultforwarder.Module,
		fx.Supply(demultiplexer.Params{}),
		fx.Supply(defaultforwarder.Params{}),
		core.MockBundle,
	)
}

func TestMockBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, MockBundle,
		core.MockBundle,
		fx.Provide(func() *ddagg.AgentDemultiplexer {
			return &ddagg.AgentDemultiplexer{}
		}),
	)
}

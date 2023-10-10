// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package ndmtmp

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	ddagg "github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle,
		fx.Provide(func() *ddagg.AgentDemultiplexer {
			return &ddagg.AgentDemultiplexer{}
		}),
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/collector/collector/collectorimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle(), core.MockBundle(),
		fx.Supply(optional.NewNoneOption[runnerimpl.MetadataProvider]()),
		fx.Provide(func() serializer.MetricSerializer { return nil }),
		collectorimpl.MockModule(),
		fx.Provide(func() optional.Option[agent.Component] {
			return optional.NewNoneOption[agent.Component]()
		}),
	)
}

func TestMockBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, MockBundle())
}

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
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle(), core.MockBundle(),
		fx.Supply(option.None[runnerimpl.MetadataProvider]()),
		fx.Provide(func() serializer.MetricSerializer { return nil }),
		collectorimpl.MockModule(),
		fx.Provide(func() option.Option[agent.Component] {
			return option.None[agent.Component]()
		}),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		haagentmock.Module(),
		fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
		fx.Provide(func(ipcComp ipc.Component) ipc.HTTPClient { return ipcComp.GetClient() }),
	)

}

func TestMockBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, MockBundle())
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"context"
	"testing"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	coreStatusImpl "github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"
	remotetraceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/fx-remote"
	"github.com/DataDog/datadog-agent/comp/process/runner"
	"github.com/DataDog/datadog-agent/comp/process/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/process/types"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

var mockCoreBundleParams = core.BundleParams{
	LogParams: log.ForOneShot("PROCESS", "trace", false),
}

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle(),
		fx.Supply(mockCoreBundleParams),
		fx.Provide(func() types.CheckComponent { return nil }),
		core.MockBundle(),
		hostnameimpl.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		fx.Provide(func() tagger.Component { return taggerfxmock.SetupFakeTagger(t) }),
		coreStatusImpl.Module(),
		settingsimpl.MockModule(),
		statusimpl.Module(),
		fx.Supply(
			status.Params{
				PythonVersionGetFunc: python.GetPythonVersion,
			},
		),
		fx.Provide(func() context.Context { return context.TODO() }),
		rdnsquerier.MockModule(),
		npcollectorimpl.MockModule(),
		statsd.MockModule(),
		fx.Provide(func() ddgostatsd.ClientInterface {
			return &ddgostatsd.NoOpClient{}
		}),
		fx.Provide(func() secrets.Component { return secretsmock.New(t) }),
		fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
		fx.Provide(func(ipcComp ipc.Component) ipc.HTTPClient { return ipcComp.GetClient() }),
	)
}

func TestBundleOneShot(t *testing.T) {
	runCmd := func(r runner.Component) {
		checks := r.GetProvidedChecks()
		require.Len(t, checks, 5)

		var names []string
		for _, c := range checks {
			c := c.Object()
			names = append(names, c.Name())
		}
		require.ElementsMatch(t, []string{
			"process",
			"container",
			"rtcontainer",
			"connections",
			"process_discovery",
		}, names)
	}

	err := fxutil.OneShot(runCmd,
		fx.Supply(
			fx.Annotate(t, fx.As(new(testing.TB))),

			mockCoreBundleParams,
		),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			return config.NewMockWithOverrides(t, map[string]interface{}{"hostname": "testhost"})
		}),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		fx.Provide(func() tagger.Component { return taggerfxmock.SetupFakeTagger(t) }),
		eventplatformreceiverimpl.Module(),
		eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),
		rdnsquerier.MockModule(),
		remotetraceroute.Module(),
		npcollectorimpl.Module(),
		statsd.MockModule(),
		fx.Provide(func() ddgostatsd.ClientInterface {
			return &ddgostatsd.NoOpClient{}
		}),
		fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
		fx.Provide(func(ipcComp ipc.Component) ipc.HTTPClient { return ipcComp.GetClient() }),
		fx.Provide(func() secrets.Component { return secretsmock.New(t) }),
		sysprobeconfigimpl.MockModule(),
		telemetryimpl.MockModule(),
		hostnameimpl.MockModule(),
		Bundle(),
	)
	require.NoError(t, err)
}

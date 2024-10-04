// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/flare/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

func TestFlareCreation(t *testing.T) {
	realProvider := func(_ types.FlareBuilder) error { return nil }

	f := newFlare(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			config.MockModule(),
			secretsimpl.MockModule(),
			nooptelemetry.Module(),
			fx.Provide(func() diagnosesendermanager.Component { return nil }),
			fx.Provide(func() Params { return Params{} }),
			collector.NoneModule(),
			fx.Supply(optional.NewNoneOption[workloadmeta.Component]()),
			fx.Supply(optional.NewNoneOption[autodiscovery.Component]()),
			// provider a nil FlareCallback
			fx.Provide(fx.Annotate(
				func() types.FlareCallback { return nil },
				fx.ResultTags(`group:"flare"`),
			)),
			// provider a real FlareCallback
			fx.Provide(fx.Annotate(
				func() types.FlareCallback { return realProvider },
				fx.ResultTags(`group:"flare"`),
			)),
		),
	)

	assert.Len(t, f.Comp.(*flare).providers, 1)
	assert.NotNil(t, f.Comp.(*flare).providers[0])
}

func TestRunProviders(t *testing.T) {
	deps := fxutil.Test[dependencies](
		t,
		fx.Provide(func() log.Component { return logmock.New(t) }),
		config.MockModule(),
		secretsimpl.MockModule(),
		nooptelemetry.Module(),
		fx.Provide(func() diagnosesendermanager.Component { return nil }),
		fx.Provide(func() Params { return Params{} }),
		collector.NoneModule(),
		fx.Supply(optional.NewNoneOption[workloadmeta.Component]()),
		fx.Supply(optional.NewNoneOption[autodiscovery.Component]()),
		// provider a nil FlareCallback
		fx.Provide(fx.Annotate(
			func() types.FlareCallback { return nil },
			fx.ResultTags(`group:"flare"`),
		)),
	)
	deps.Config.Set("flare_provider_timeout", 1, model.SourceAgentRuntime)
	f := newFlare(deps)

	var firstRan atomic.Bool
	var secondRan atomic.Bool
	var secondDone atomic.Bool
	providers := []types.FlareCallback{
		func(_ types.FlareBuilder) error {
			firstRan.Store(true)
			return nil
		},
		func(_ types.FlareBuilder) error {
			secondRan.Store(true)
			time.Sleep(10 * time.Second)
			secondRan.Store(true)
			return nil
		},
	}

	fb, err := helpers.NewFlareBuilder(false)
	require.NoError(t, err)
	f.Comp.(*flare).runProviders(fb, providers)
	require.True(t, firstRan.Load())
	require.True(t, secondRan.Load())
	require.False(t, secondDone.Load())
}

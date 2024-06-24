// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestFlareCreation(t *testing.T) {
	realProvider := func(fb types.FlareBuilder) error { return nil }

	f, _ := newFlare(
		fxutil.Test[dependencies](
			t,
			logimpl.MockModule(),
			settingsimpl.MockModule(),
			config.MockModule(),
			secretsimpl.MockModule(),
			fx.Provide(func(secretMock secrets.Mock) secrets.Component {
				component := secretMock.(secrets.Component)
				return component
			}),
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package languagedetection

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/languagedetection/client"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		// instantiate all of the language detection components, since this is not done
		// automatically.
		config.Module(),
		fx.Supply(config.Params{}),
		telemetryimpl.Module(),
		logimpl.Module(),
		fx.Provide(func(secretResolver secrets.Component) optional.Option[secrets.Component] {
			return optional.NewOption[secrets.Component](secretResolver)
		}),
		secretsimpl.MockModule(),
		fx.Supply(logimpl.Params{}),
		workloadmetafx.Module(),
		fx.Supply(workloadmeta.NewParams()),
		fx.Invoke(func(client.Component) {}),
		Bundle(),
	))
}

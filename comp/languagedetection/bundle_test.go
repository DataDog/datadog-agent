// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package languagedetection

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/languagedetection/client"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		// instantiate all of the language detection components, since this is not done
		// automatically.
		config.Module,
		fx.Supply(config.Params{}),
		telemetry.Module,
		log.Module,
		fx.Provide(func() secrets.Component { return secretsimpl.NewMock() }),
		secretsimpl.MockModule,
		fx.Supply(log.Params{}),
		workloadmeta.Module,
		fx.Supply(workloadmeta.NewParams()),
		fx.Invoke(func(client.Component) {}),
		Bundle,
	))
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp
// +build otlp

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	corecomp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestFullYamlConfigWithOTLP(t *testing.T) {

	fxutil.Test(t, fx.Options(
		fx.Supply(corecomp.NewAgentParamsWithSecrets("./testdata/full.yaml")),
		corecomp.MockModule,
		fx.Supply(Params{}),
		MockModule,
	), func(config Component) {
		cfg := config.Object()

		require.NotNil(t, cfg)

		assert.Equal(t, "0.0.0.0", cfg.OTLPReceiver.BindHost)
		assert.Equal(t, 50053, cfg.OTLPReceiver.GRPCPort)
	})
}

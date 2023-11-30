// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	corecomp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestSetupConfig(t *testing.T) {
	config := fxutil.Test[corecomp.Component](t, fx.Options(
		corecomp.MockModule,
		fx.Replace(corecomp.MockParams{
			Params:      corecomp.Params{ConfFilePath: "./testdata/full.yaml"},
			SetupConfig: true,
		}),
	))
	assert.Equal(t, Datadog.Get("api_key"), config.Get("api_key"))
}

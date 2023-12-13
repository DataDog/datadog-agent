// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package agent

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestBuildEndpoints(t *testing.T) {
	config := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule,
	))

	endpoints, err := buildEndpoints(config)
	assert.Nil(t, err)
	assert.Equal(t, "agent-intake.logs.datadoghq.com", endpoints.Main.Host)
}

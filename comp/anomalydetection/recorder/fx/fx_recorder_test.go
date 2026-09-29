// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder && python

package fx

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestTaggedModuleWithoutProvider(t *testing.T) {
	var component option.Option[recorder.Component]
	var provider option.Option[recorder.WriterFactory]
	app := fx.New(fx.NopLogger, Module().Option, fx.Populate(&component, &provider))
	require.NoError(t, app.Err())
	_, hasComponent := component.Get()
	_, hasProvider := provider.Get()
	require.False(t, hasComponent)
	require.False(t, hasProvider)
}

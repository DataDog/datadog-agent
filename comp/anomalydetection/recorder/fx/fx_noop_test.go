// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !anomalydetection_recorder || !python

package fx

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestNoopModuleProvidesNoRecorder(t *testing.T) {
	var provided option.Option[recorder.Component]
	app := fx.New(fx.NopLogger, Module().Option, fx.Populate(&provided))
	require.NoError(t, app.Err())
	_, present := provided.Get()
	require.False(t, present)
}

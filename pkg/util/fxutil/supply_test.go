// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package fxutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

// paramsWithUnexported has unexported fields — the common case for component Params.
type paramsWithUnexported struct {
	fromCLI bool
	logFile string
}

// paramsWithExported has only exported fields.
type paramsWithExported struct {
	Timeout int
	Name    string
}

func TestSupply_ExportedFields(t *testing.T) {
	want := paramsWithExported{Timeout: 30, Name: "test"}
	var got paramsWithExported

	app := fxtest.New(t,
		Supply(want),
		fx.Invoke(func(p paramsWithExported) { got = p }),
	)
	require.NoError(t, app.Err())
	app.RequireStart().RequireStop()

	assert.Equal(t, want, got)
}

func TestSupply_UnexportedFields(t *testing.T) {
	want := paramsWithUnexported{fromCLI: true, logFile: "/tmp/test.log"}
	var got paramsWithUnexported

	app := fxtest.New(t,
		Supply(want),
		fx.Invoke(func(p paramsWithUnexported) { got = p }),
	)
	require.NoError(t, app.Err())
	app.RequireStart().RequireStop()

	assert.Equal(t, want, got)
}

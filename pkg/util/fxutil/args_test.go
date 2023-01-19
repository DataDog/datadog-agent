// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func TestDelayedFxInvocationNoReturn(t *testing.T) {
	var got string
	fn := func(str string) {
		got = str
	}
	delayed := newDelayedFxInvocation(fn)

	app := fxtest.New(t,
		fx.Provide(func() string { return "a string" }),
		delayed.option(),
	)
	defer app.RequireStart().RequireStop()

	require.Equal(t, got, "") // not gotten yet
	require.NoError(t, delayed.call())
	require.Equal(t, got, "a string")
}

func TestDelayedFxInvocationErrorReturn(t *testing.T) {
	var got string
	fn := func(str string) error {
		got = str
		return errors.New("uhoh")
	}
	delayed := newDelayedFxInvocation(fn)

	app := fxtest.New(t,
		fx.Provide(func() string { return "a string" }),
		delayed.option(),
	)
	defer app.RequireStart().RequireStop()

	require.Equal(t, got, "")
	require.ErrorContains(t, delayed.call(), "uhoh")
	require.Equal(t, got, "a string")
}

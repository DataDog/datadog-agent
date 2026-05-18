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

type supplyTestInterface interface {
	Hello() string
}

type supplyTestImpl struct{ msg string }

func (s *supplyTestImpl) Hello() string { return s.msg }

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

// TestSupply_Interface verifies that when T is an interface, Supply provides
// the declared interface type — not the concrete type, which is the footgun
// in raw fx.Supply.
func TestSupply_Interface(t *testing.T) {
	impl := &supplyTestImpl{msg: "hi"}
	var got supplyTestInterface

	// Supply[supplyTestInterface] must provide supplyTestInterface, not *supplyTestImpl.
	app := fxtest.New(t,
		Supply[supplyTestInterface](impl),
		fx.Invoke(func(i supplyTestInterface) { got = i }),
	)
	require.NoError(t, app.Err())
	app.RequireStart().RequireStop()

	assert.Equal(t, "hi", got.Hello())
}

// TestSupply_Interface_RawFxBehavior documents the footgun in fx.Supply:
// providing a concrete value stored in an interface variable gives the
// concrete type, not the interface — so fx.Invoke(func(supplyTestInterface))
// would fail to satisfy the dependency.
func TestSupply_Interface_RawFxBehavior(t *testing.T) {
	var impl supplyTestInterface = &supplyTestImpl{msg: "hi"}

	// Raw fx.Supply uses reflect.TypeOf(impl) == *supplyTestImpl, not supplyTestInterface.
	// The graph provides *supplyTestImpl, so injecting supplyTestInterface fails.
	app := fx.New(
		fx.Supply(impl),
		fx.Invoke(func(supplyTestInterface) {}),
	)
	assert.Error(t, app.Err(), "fx.Supply on an interface value provides the concrete type, not the interface")
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

type testDeps struct {
	fx.In
	S string
}

func TestTest(t *testing.T) {
	deps := Test[testDeps](t, fx.Options(
		fx.Provide(func() string { return "a string!" }),
	))
	require.Equal(t, "a string!", deps.S)
}

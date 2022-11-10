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

func TestTest(t *testing.T) {
	Test(t, fx.Options(
		fx.Provide(func() string { return "a string!" }),
	), func(s string) {
		require.Equal(t, "a string!", s)
	})
}

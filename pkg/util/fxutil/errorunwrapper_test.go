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
)

func TestUnwrapIfErrArgumentsFailed(t *testing.T) {
	expectedError := errors.New("expected error")
	err := OneShot(func(*struct{}) {},
		fx.Provide(func() (*struct{}, error) { return nil, expectedError }),
	)
	require.Equal(t, expectedError.Error(), err.Error())
}

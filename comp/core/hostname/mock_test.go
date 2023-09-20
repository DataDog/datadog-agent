// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package hostname

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestMockGet(t *testing.T) {
	s := fxutil.Test[Component](t, MockModule)
	name, err := s.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "my-hostname", name)
}

func TestMockGetWithProvider(t *testing.T) {
	s := fxutil.Test[Component](t, MockModule)
	data, err := s.GetWithProvider(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "my-hostname", data.Hostname)
	assert.Equal(t, "mockService", data.Provider)
}

func TestMockProvide(t *testing.T) {
	s := fxutil.Test[Component](t,
		MockModule,
		fx.Replace(MockHostname("foo")),
	)
	assert.Equal(t, "foo", s.GetSafe(context.Background()))
}

func TestMockSet(t *testing.T) {
	s := fxutil.Test[Mock](t,
		MockModule,
	)
	s.Set("bar")
	assert.Equal(t, "bar", s.GetSafe(context.Background()))
}

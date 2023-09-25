// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package resources

import (
	"testing"

	"go.uber.org/fx"
)

type mockDependencies struct {
	fx.In

	Params MockParams
}

type mock struct {
	data map[string]interface{}
}

func (m *mock) Get() map[string]interface{} {
	return m.data
}

func newMock(deps mockDependencies, t testing.TB) Component {
	return &mock{
		data: deps.Params.Data,
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package apiimpl

import (
	"net"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/api"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockApiServer struct {
	name string
}

var _ api.Mock = (*mockApiServer)(nil)

// MockModule defines the fx options for the mock component.
var MockModule = fxutil.Component(
	fx.Provide(newMock),
)

func newMock() api.Mock {
	return &mockApiServer{}
}

// ServerAddress retruns the server address.
func (mock *mockApiServer) ServerAddress() *net.TCPAddr {
	return nil
}

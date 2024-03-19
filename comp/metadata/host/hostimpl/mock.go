// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package hostimpl

import (
	"context"

	hostinterface "github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}

type MockHost struct{}

func (h *MockHost) GetPayloadAsJSON(ctx context.Context) ([]byte, error) {
	str := "some bytes"
	return []byte(str), nil
}

func newMock() hostinterface.Component {
	return &MockHost{}
}

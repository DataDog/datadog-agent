// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package packagesigningimpl

import (
	psinterface "github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// MockModule defines the fx options for the mocked component
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}

// MockPkgSigning is the mocked struct that implements the packagesigning component interface
type MockPkgSigning struct{}

// GetAsJSON is a mocked method on the component
func (h *MockPkgSigning) GetAsJSON() ([]byte, error) {
	str := "some bytes"
	return []byte(str), nil
}

// newMock returns the mocked packagesigning struct
func newMock() psinterface.Component {
	return &MockPkgSigning{}
}

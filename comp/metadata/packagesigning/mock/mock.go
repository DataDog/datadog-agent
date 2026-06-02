// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the packagesigning component
package mock

import (
	packagesigning "github.com/DataDog/datadog-agent/comp/metadata/packagesigning/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Mock implements mock-specific methods for the packagesigning component.
type Mock interface {
	packagesigning.Component
}

// MockProvides is the mock component output
type MockProvides struct {
	Comp packagesigning.Component
}

// MockPkgSigning is the mocked struct that implements the packagesigning component interface
type MockPkgSigning struct{}

// MockModule defines the fx options for the mocked component
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(newMock),
	)
}

// newMock returns the mocked packagesigning struct
func newMock() MockProvides {
	ps := &MockPkgSigning{}
	return MockProvides{
		Comp: ps,
	}
}

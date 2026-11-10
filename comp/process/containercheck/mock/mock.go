// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock implements a mock for the containercheck component.
package mock

import (
	"testing"

	"github.com/stretchr/testify/mock"

	containercheck "github.com/DataDog/datadog-agent/comp/process/containercheck/def"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/checks/mocks"
)

var _ containercheck.Component = (*Mock)(nil)

// Mock implements a mock containercheck component.
type Mock struct {
	check checks.Check
}

// Object returns the underlying check.
func (m *Mock) Object() checks.Check {
	return m.check
}

// New creates a new mock containercheck component for testing.
func New(t testing.TB) containercheck.Component {
	c := mocks.NewCheck(t)
	c.On("Init", mock.Anything, mock.Anything, mock.AnythingOfType("bool")).Return(nil).Maybe()
	c.On("Name").Return("container").Maybe()
	c.On("SupportsRunOptions").Return(false).Maybe()
	c.On("Realtime").Return(false).Maybe()
	c.On("Cleanup").Maybe()
	c.On("Run", mock.Anything, mock.Anything).Return(&checks.StandardRunResult{}, nil).Maybe()
	c.On("ShouldSaveLastRun").Return(false).Maybe()
	c.On("IsEnabled").Return(true).Maybe()
	return &Mock{check: c}
}

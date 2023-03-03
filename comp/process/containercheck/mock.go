// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containercheck

import (
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/checks/mocks"
)

var _ types.CheckComponent = (*mockCheck)(nil)

type mockCheck struct {
	mock *mocks.Check
}

func (m *mockCheck) Object() checks.Check {
	return m.mock
}

func newMock(t testing.TB, params types.MockCheckParams[*checks.ContainerCheck]) types.ProvidesCheck {
	c := mocks.NewCheck(t)
	if params.OrchestrateMock == nil {
		c.On("Init", mock.Anything, mock.Anything).Return(nil).Maybe()
		c.On("Name").Return("container").Maybe()
		c.On("SupportsRunOptions").Return(false).Maybe()
		c.On("Realtime").Return(false).Maybe()
		c.On("Cleanup").Maybe()
		c.On("Run", mock.Anything, mock.Anything).Return(&checks.StandardRunResult{}, nil).Maybe()
		c.On("ShouldSaveLastRun").Return(false).Maybe()
		c.On("IsEnabled").Return(true).Maybe()
	} else {
		params.OrchestrateMock(c)
	}
	return types.ProvidesCheck{
		CheckComponent: &mockCheck{
			mock: c,
		},
	}
}

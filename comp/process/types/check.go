// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package types

import (
	"go.uber.org/fx"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	checkMocks "github.com/DataDog/datadog-agent/pkg/process/checks/mocks"
)

// Payload defines payload from the check
type Payload struct {
	CheckName string
	Message   []model.MessageBody
}

// CheckComponent defines an interface implemented by checks
type CheckComponent interface {
	Object() checks.Check
}

// ProvidesCheck wraps a check implementation for consumption in components
type ProvidesCheck struct {
	fx.Out

	CheckComponent CheckComponent `group:"check"`
}

//nolint:revive // TODO(PROC) Fix revive linter
type MockCheckParams[T checks.Check] struct {
	fx.In

	OrchestrateMock func(mock *checkMocks.Check) `optional:"true"`
}

//nolint:revive // TODO(PROC) Fix revive linter
type RTResponse []*model.CollectorStatus

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

//nolint:revive // TODO(PROC) Fix revive linter
package types

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/process/checks"
	checkMocks "github.com/DataDog/datadog-agent/pkg/process/checks/mocks"
)

//nolint:revive // TODO(PROC) Fix revive linter
type MockCheckParams[T checks.Check] struct {
	fx.In

	OrchestrateMock func(mock *checkMocks.Check) `optional:"true"`
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package fx provides the fx module for the batchsender mock component
package fx

import (
	batchsendermock "github.com/DataDog/datadog-agent/comp/logs-library/api/batchsender/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(batchsendermock.NewMock),
	)
}

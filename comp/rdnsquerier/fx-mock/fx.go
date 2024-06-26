// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fxmock provides the fxmock module for the rdnsquerier component
package fxmock

import (
	rdnsqueriermock "github.com/DataDog/datadog-agent/comp/rdnsquerier/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			rdnsqueriermock.NewMock,
		),
	)
}

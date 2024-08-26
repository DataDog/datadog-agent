// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the demultiplexerendpoint component
package fx

import (
	demultiplexerendpointmock "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexerendpoint/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for this mock component
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			demultiplexerendpointmock.NewMock,
		),
	)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fxmock provides the fx module for the datadogclient component
package fxmock

import (
	datadogclientmock "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule provides the mock integrations component to fx
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			datadogclientmock.NewMock,
		),
	)
}

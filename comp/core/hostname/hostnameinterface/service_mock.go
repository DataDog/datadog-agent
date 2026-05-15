// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock instead.
package hostnameinterface

import (
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockHostname is an alias for injecting a mock hostname.
//
// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock.MockHostname instead.
type MockHostname = hostnamemock.MockHostname

// MockModule defines the fx options for the mock component.
//
// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock.MockModule instead.
func MockModule() fxutil.Module {
	return hostnamemock.MockModule()
}

// NewMock returns a new instance of the mock for the component hostname.
//
// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock.NewMock instead.
var NewMock = hostnamemock.NewMock

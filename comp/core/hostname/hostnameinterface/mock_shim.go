// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package hostnameinterface

import mockpkg "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"

// Mock implements mock-specific methods.
// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock.Mock instead.
type Mock = mockpkg.Mock

// MockHostname is an alias for injecting a mock hostname.
// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock.MockHostname instead.
type MockHostname = mockpkg.MockHostname

// MockModule defines the fx options for the mock component.
// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock.MockModule instead.
var MockModule = mockpkg.MockModule

// NewMock returns a new instance of the mock for the component hostname.
// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock.NewMock instead.
var NewMock = mockpkg.NewMock

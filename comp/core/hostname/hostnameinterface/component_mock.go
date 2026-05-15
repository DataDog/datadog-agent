// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock instead.
package hostnameinterface

import hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"

// Mock implements mock-specific methods.
//
// Deprecated: use github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock.Mock instead.
type Mock = hostnamemock.Mock

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides the mock component for serializer/compression
package mock

import compression "github.com/DataDog/datadog-agent/comp/serializer/compression/def"

// Mock implements mock-specific methods.
type Mock interface {
	compression.Component
}

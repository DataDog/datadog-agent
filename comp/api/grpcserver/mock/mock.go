// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the grpc component
package mock

import (
	"testing"

	grpc "github.com/DataDog/datadog-agent/comp/api/grpcserver/def"
)

// Mock returns a mock for grpc component.
func Mock(_ *testing.T) grpc.Component {
	// TODO: Implement the grpc mock
	return nil
}

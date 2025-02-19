// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package mock

import (
	"testing"

	grpcClient "github.com/DataDog/datadog-agent/comp/core/grpcClient/def"
)

// Mock returns a mock for grpcClient component.
func Mock(t *testing.T) grpcClient.Component {
	// TODO: Implement the grpcClient mock
	return nil
}

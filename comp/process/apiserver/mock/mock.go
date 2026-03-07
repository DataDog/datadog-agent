// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the apiserver component.
package mock

import (
	"testing"

	apiserver "github.com/DataDog/datadog-agent/comp/process/apiserver/def"
)

type mockApiServer struct{}

// Mock returns a mock for apiserver component.
func Mock(_ *testing.T) apiserver.Component {
	return &mockApiServer{}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the statsd component.
package mock

import (
	"testing"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	statsd "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/def"
)

// Mock returns a mock for statsd component.
func Mock(_ *testing.T) statsd.Component {
	return &ddgostatsd.NoOpClient{}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the ssi component
package mock

import (
	"testing"

	ssi "github.com/DataDog/datadog-agent/comp/trace/ssi/def"
)

// Mock returns a mock for ssi component.
// If the comp doesn't have any public method the component does not need a mock
// TODO: Implement or remove the mock depending of your needs
func Mock(_ *testing.T) ssi.Component {
	return nil
}

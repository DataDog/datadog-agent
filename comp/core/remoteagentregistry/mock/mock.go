// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the remoteagentregistry component
package mock

import (
	"testing"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
)

// Mock returns a mock for remoteagentregistry component.
func Mock(_ *testing.T) remoteagentregistry.Component {
	return nil
}

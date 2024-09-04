// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock of the lsof component
package mock

import (
	"testing"

	lsof "github.com/DataDog/datadog-agent/comp/core/lsof/def"
)

// Mock returns a mock for lsof component.
func Mock(*testing.T) lsof.Component {
	return struct{}{}
}

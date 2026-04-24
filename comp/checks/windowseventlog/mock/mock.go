// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the windowseventlog component.
package mock

import (
	"testing"

	windowseventlog "github.com/DataDog/datadog-agent/comp/checks/windowseventlog/def"
)

// Mock returns a mock for the windowseventlog component.
func Mock(_ *testing.T) windowseventlog.Component {
	return struct{}{}
}

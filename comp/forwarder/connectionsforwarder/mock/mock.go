// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the connectionsforwarder component
package mock

import (
	"testing"

	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
)

// Mock returns a mock for connectionsforwarder component.
func Mock(_ *testing.T) connectionsforwarder.Component {
	return &defaultforwarder.MockedForwarder{}
}

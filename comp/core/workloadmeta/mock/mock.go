// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package mock provides the workloadmeta mock component for the Datadog Agent
package mock

import (
	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// Mock implements mock-specific methods.
type Mock interface {
	wmdef.Component

	// The following are for testing purposes and should maybe be revisited

	// Set allows setting an entity in the workloadmeta store
	Set(entity wmdef.Entity)

	// Unset removes an entity from the workloadmeta store
	Unset(entity wmdef.Entity)

	// GetConfig returns a Config Reader for the internal injected config
	GetConfig() config.Reader
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package gui provides the GUI server component for the Datadog Agent.
package gui

// team: agent-shared-components

// Component is the component type.
type Component interface {
	GetCSRFToken() string
}

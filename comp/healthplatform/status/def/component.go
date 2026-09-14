// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package status defines the interface for the health platform status component.
package status

// team: fleet-remediation

// Component is the top-level marker interface required by the component linter.
// The health platform status component has no public methods: its only
// purpose is to register an `agent status` information provider, which it
// does as a side effect of being constructed.
type Component = any

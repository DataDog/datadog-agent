//go:build !windows

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package publishermetadatacache provides a cache for Windows Event Log publisher metadata handles
package publishermetadatacache

// Component is a no-op stub for non-Windows platforms
type Component interface {
	// Stub method - not implemented on non-Windows platforms
}
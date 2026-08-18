// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix && !darwin

package darwin

// newNameResolver returns no resolver off darwin. This package's translation and
// rule tests build on any unix so they run in Linux CI, but user and group name
// resolution is a macOS concern: pkg/security/resolvers/usergroup has a different
// constructor on Linux, and nothing here needs it.
func newNameResolver() (nameResolver, error) {
	return nil, nil
}

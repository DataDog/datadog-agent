// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package landlock

import "fmt"

// Restrict fails closed because Landlock is a Linux-only security boundary.
func Restrict(_ []string) error {
	return fmt.Errorf("%w on this platform", ErrUnsupported)
}

// RestrictWithTrustedPaths fails closed because Landlock is a Linux-only
// security boundary.
func RestrictWithTrustedPaths(_ []string, _ []TrustedPath) error {
	return fmt.Errorf("%w on this platform", ErrUnsupported)
}

// RestrictReadOnlyWithTrustedPaths fails closed because Landlock is a
// Linux-only security boundary.
func RestrictReadOnlyWithTrustedPaths(_ []string, _ []TrustedPath) error {
	return fmt.Errorf("%w on this platform", ErrUnsupported)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package seccomp

// Restrict cannot install a seccomp filter outside Linux. It returns an error
// rather than silently running the privileged worker without this boundary.
func Restrict(_ []string) error {
	return ErrUnsupported
}

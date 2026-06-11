// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build (windows && npm) || darwin

package modules

// categorizeTracerError returns err unchanged on platforms where eBPF verifier and USM
// failures cannot occur (Windows, Darwin).
func categorizeTracerError(err error) error {
	return err
}

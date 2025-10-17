// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

// Package tags provides gpu host tags to the host payload
package tags

// This is a dummy entry point into the module to avoid adding the gpu_host tags as host tags when running unit tests.
// This prevent unit tests from failing depending on the type of host they're running on.

// GetTags returns a slice of tags indicating GPU presence
func GetTags() []string {
	return nil
}

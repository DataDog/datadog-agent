// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package user provides helpers to change the user of the process.
package user

// IsRoot always returns true on windows.
func IsRoot() bool {
	return true
}

// RootToDatadogAgent is a noop on windows.
func RootToDatadogAgent() error {
	return nil
}

// DatadogAgentToRoot is a noop on windows.
func DatadogAgentToRoot() error {
	return nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package driver TODO comment
package driver

import "github.com/DataDog/datadog-agent/cmd/system-probe/config"

// Init exported function should have comment or be unexported
func Init(cfg *config.Config) error {
	return nil
}

// IsNeeded exported function should have comment or be unexported
func IsNeeded() bool {
	// return true, so no stop attempts are made
	return true
}

// Start exported function should have comment or be unexported
func Start() error {
	return nil
}

// Stop exported function should have comment or be unexported
func Stop() error {
	return nil
}

// ForceStop exported function should have comment or be unexported
func ForceStop() error {
	return nil
}

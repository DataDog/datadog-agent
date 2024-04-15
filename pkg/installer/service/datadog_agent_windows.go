// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package service provides a way to interact with os services
package service

// SetupAgentUnits noop
func SetupAgentUnits() error {
	return nil
}

// StartAgentExperiment noop
func StartAgentExperiment() error {
	return nil
}

// StopAgentExperiment noop
func StopAgentExperiment() error {
	return nil
}

// RemoveAgentUnits noop
func RemoveAgentUnits() {}

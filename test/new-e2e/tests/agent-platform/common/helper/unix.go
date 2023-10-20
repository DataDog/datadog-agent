// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package helper implement interfaces to get some information that can be OS specific
package helper

// Unix implement helper function for Unix distributions
type Unix struct{}

// NewUnixHelper create a new instance of Unix helper
func NewUnixHelper() *Unix { return &Unix{} }

// GetInstallFolder return the install folder path
func (u *Unix) GetInstallFolder() string { return "/opt/datadog-agent/" }

// GetConfigFolder return the config folder path
func (u *Unix) GetConfigFolder() string { return "/etc/datadog-agent/" }

// GetBinaryPath return the datadog-agent binary path
func (u *Unix) GetBinaryPath() string { return "/usr/bin/datadog-agent" }

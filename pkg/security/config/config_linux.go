// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package config holds config related files
package config

func (c *RuntimeSecurityConfig) sanitizePlatform() {
	// Force the disable of features unavailable on EBPFLess
	if c.EBPFLessEnabled {
		c.ActivityDumpEnabled = false
		c.SecurityProfileEnabled = false
	}
}

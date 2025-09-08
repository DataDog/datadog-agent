// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common holds common related files
package common

import "strings"

// GetFamilyAddress returns the address famility to use for system-probe <-> security-agent communication
func GetFamilyAddress(path string) string {
	if strings.HasPrefix(path, "/") {
		return "unix"
	}
	return "tcp"
}

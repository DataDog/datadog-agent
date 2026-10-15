// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"os"
	"path/filepath"
	"regexp"
)

// LockdownMode defines a lockdown type
type LockdownMode string

const (
	// None mode
	None LockdownMode = "none"
	// Integrity mode
	Integrity LockdownMode = "integrity"
	// Confidentiality mode
	Confidentiality LockdownMode = "confidentiality"
	// Unknown mode
	Unknown LockdownMode = "unknown"
)

// SecurityFSHostRootPath is where securityfs can be reached through the host root mount,
// when it is not mounted explicitly in the container
const SecurityFSHostRootPath = "/host/root/sys/kernel/security"

var re = regexp.MustCompile(`\[(.*)\]`)

func getLockdownMode(data string) LockdownMode {
	mode := re.FindString(data)

	switch mode {
	case "[none]":
		return None
	case "[integrity]":
		return Integrity
	case "[confidentiality]":
		return Confidentiality
	}
	return Unknown
}

// GetLockdownMode returns the lockdown
func GetLockdownMode() LockdownMode {
	data, err := os.ReadFile(filepath.Join(SysFSRoot(), "kernel/security/lockdown"))
	if err != nil {
		if data, err = os.ReadFile(filepath.Join(SecurityFSHostRootPath, "lockdown")); err != nil {
			return Unknown
		}
	}

	return getLockdownMode(string(data))
}

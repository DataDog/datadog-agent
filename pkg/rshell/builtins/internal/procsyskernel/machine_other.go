// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package procsyskernel

// ReadMachine returns the machine hardware name (uname -m) from
// sys/kernel/arch.
func ReadMachine(procPath string) (string, error) {
	return ReadFile(procPath, "arch")
}

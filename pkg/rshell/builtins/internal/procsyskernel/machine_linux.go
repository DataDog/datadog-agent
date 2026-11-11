// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procsyskernel

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// ReadMachine returns the machine hardware name (uname -m). It reads
// sys/kernel/arch, which Linux only provides since 6.1. When that file is
// absent (e.g. RHEL 9's 5.14 kernel) it falls back to the machine field of
// uname(2), the same source GNU uname uses. Any other read error is returned.
func ReadMachine(procPath string) (string, error) {
	machine, err := ReadFile(procPath, "arch")
	if !errors.Is(err, os.ErrNotExist) {
		return machine, err
	}
	var uts unix.Utsname
	if unameErr := unix.Uname(&uts); unameErr != nil {
		return "", err
	}
	return unix.ByteSliceToString(uts.Machine[:]), nil
}

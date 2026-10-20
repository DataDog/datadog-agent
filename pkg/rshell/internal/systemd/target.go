// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package systemd contains the trusted target and transport implementation
// used by systemd-aware builtins.
package systemd

import (
	"fmt"
	"path/filepath"
	"strings"
)

const MaxJournalDirs = 8

// Target identifies one systemd host through journal paths supplied by the
// embedding application.
type Target struct {
	JournalDirs          []string
	MachineIDPath        string
	JournalControlSocket string
	ManagerBusSocket     string
}

// LocalTarget returns the standard paths for the local systemd host.
func LocalTarget() Target {
	return Target{
		JournalDirs: []string{
			filepath.FromSlash("/var/log/journal"),
			filepath.FromSlash("/run/log/journal"),
		},
		MachineIDPath:        filepath.FromSlash("/etc/machine-id"),
		JournalControlSocket: filepath.FromSlash("/run/systemd/journal/io.systemd.journal"),
		ManagerBusSocket:     filepath.FromSlash("/run/dbus/system_bus_socket"),
	}
}

// ResolveTarget validates and resolves a target. A zero target selects the
// local host. Once any explicit field is supplied, omitted fields remain empty
// and never fall back to local paths.
func ResolveTarget(target Target) (Target, error) {
	if len(target.JournalDirs) == 0 && target.MachineIDPath == "" && target.JournalControlSocket == "" && target.ManagerBusSocket == "" {
		return LocalTarget(), nil
	}
	if len(target.JournalDirs) > MaxJournalDirs {
		return Target{}, fmt.Errorf("systemd target has %d journal directories; maximum is %d", len(target.JournalDirs), MaxJournalDirs)
	}
	if target.MachineIDPath == "" {
		return Target{}, fmt.Errorf("systemd target machine ID path is required with explicit paths")
	}

	resolved := Target{JournalDirs: make([]string, 0, len(target.JournalDirs))}
	seenDirs := make(map[string]struct{}, len(target.JournalDirs))
	for i, dir := range target.JournalDirs {
		clean, err := validateAbsolutePath(fmt.Sprintf("journal directory %d", i), dir)
		if err != nil {
			return Target{}, err
		}
		if _, exists := seenDirs[clean]; exists {
			continue
		}
		seenDirs[clean] = struct{}{}
		resolved.JournalDirs = append(resolved.JournalDirs, clean)
	}

	var err error
	if resolved.MachineIDPath, err = validateAbsolutePath("machine ID path", target.MachineIDPath); err != nil {
		return Target{}, err
	}
	if resolved.JournalControlSocket, err = validateOptionalAbsolutePath("journal control socket", target.JournalControlSocket); err != nil {
		return Target{}, err
	}
	if resolved.ManagerBusSocket, err = validateOptionalAbsolutePath("manager bus socket", target.ManagerBusSocket); err != nil {
		return Target{}, err
	}
	return resolved, nil
}

func validateOptionalAbsolutePath(name, path string) (string, error) {
	if path == "" {
		return "", nil
	}
	return validateAbsolutePath(name, path)
}

func validateAbsolutePath(name, path string) (string, error) {
	if strings.IndexByte(path, 0) >= 0 {
		return "", fmt.Errorf("systemd target %s contains a NUL byte", name)
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("systemd target %s %q must be absolute", name, path)
	}
	return filepath.Clean(path), nil
}

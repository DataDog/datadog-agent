// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package sysctl is used to process and analyze sysctl data
package sysctl

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shirou/gopsutil/v4/cpu"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var (
	redactedContent = "******"
)

// readFileContent reads a file and processes its content based on the given rules.
func readFileContent(file string, ignoredBaseNames []string) ([]byte, error) {
	if slices.Contains(ignoredBaseNames, path.Base(file)) {
		return []byte(redactedContent), nil
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return []byte{}, err
	}

	return data, nil
}

// SnapshotEvent is a wrapper used for serialization
type SnapshotEvent struct {
	Sysctl Snapshot `json:"sysctl"`
}

// NewSnapshotEvent returns a new sysctl snapshot event
func NewSnapshotEvent(ignoredBaseNames []string) (*SnapshotEvent, error) {
	se := &SnapshotEvent{
		Sysctl: NewSnapshot(),
	}
	if err := se.Sysctl.Snapshot(ignoredBaseNames); err != nil {
		return nil, err
	}
	return se, nil
}

// ToJSON serializes the current SnapshotEvent object to JSON
func (s *SnapshotEvent) ToJSON() ([]byte, error) {
	return json.Marshal(s)
}

// Snapshot defines an internal core dump
type Snapshot struct {
	// Proc contains the /proc system control parameters and their values
	Proc map[string]interface{} `json:"proc,omitempty"`
	// Sys contains the /sys system control parameters and their values
	Sys map[string]interface{} `json:"sys,omitempty"`
	// CPUFlags contains the list of flags of the current CPU
	CPUFlags []string `json:"cpu_flags,omitempty"`
	// KernelCmdline contains the kernel command line parameters
	KernelCmdline string `json:"kernel_cmdline,omitempty"`
}

// NewSnapshot returns a new sysctl snapshot
func NewSnapshot() Snapshot {
	return Snapshot{
		Proc: make(map[string]interface{}),
		Sys:  make(map[string]interface{}),
	}
}

// Snapshot runs the snapshot by going through the filesystem
func (s *Snapshot) Snapshot(ignoredBaseNames []string) error {
	if err := s.snapshotProcSys(ignoredBaseNames); err != nil {
		return fmt.Errorf("couldn't snapshot /proc/sys: %w", err)
	}

	if err := s.snapshotSys(ignoredBaseNames); err != nil {
		return fmt.Errorf("coudln't snapshot /sys: %w", err)
	}

	if err := s.snapshotCPUFlags(); err != nil {
		return fmt.Errorf("couldn't get CPU flags: %w", err)
	}

	if err := s.snapshotKernelCmdline(ignoredBaseNames); err != nil {
		return fmt.Errorf("couldn't get kernel cmdline: %w", err)
	}
	return nil
}

// snapshotProcSys recursively reads files in /proc/sys and builds a nested JSON structure.
func (s *Snapshot) snapshotProcSys(ignoredBaseNames []string) error {
	return filepath.Walk(kernel.HostProc("/sys"), func(file string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip if mode doesn't allow reading
		mode := info.Mode()
		if mode&0444 == 0 {
			return nil
		}

		relPath, err := filepath.Rel(kernel.ProcFSRoot(), file)
		if err != nil {
			return err
		}

		value, err := readFileContent(file, ignoredBaseNames)
		if err != nil {
			return nil // Skip files that can't be read
		}

		s.InsertSnapshotEntry(s.Proc, relPath, string(value))
		return nil
	})
}

// snapshotSys reads security relevant files from the /sys filesystem
func (s *Snapshot) snapshotSys(ignoredBaseNames []string) error {
	for _, systemControl := range []string{
		"/kernel/security/lockdown",
		"/kernel/security/lsm",
	} {
		value, err := readFileContent(kernel.HostSys(systemControl), ignoredBaseNames)
		if err != nil {
			return err
		}
		s.InsertSnapshotEntry(s.Sys, systemControl, string(value))
	}

	// fetch secure boot status, ignore when missing
	_ = filepath.Walk(kernel.HostSys("/firmware/efi/efivars/"), func(file string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip if mode doesn't allow reading
		mode := info.Mode()
		if mode&0444 == 0 {
			return nil
		}

		if strings.HasPrefix(path.Base(file), "SecureBoot-") {
			// this is the secure boot file, read it now
			value, err := readFileContent(file, ignoredBaseNames)
			if err != nil {
				return err
			}
			relPath, err := filepath.Rel(kernel.SysFSRoot(), file)
			if err != nil {
				return err
			}

			secureBootValue := "Disabled"
			if len(value) > 0 && value[len(value)-1] == 1 {
				secureBootValue = "Enabled"
			}

			s.InsertSnapshotEntry(s.Sys, path.Join(path.Dir(relPath), "SecureBoot"), secureBootValue)
		}
		return nil
	})

	// add CPU vulnerabilities, ignore when missing
	_ = filepath.Walk(kernel.HostSys("/devices/system/cpu/vulnerabilities"), func(file string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip if mode doesn't allow reading
		mode := info.Mode()
		if mode&0444 == 0 {
			return nil
		}

		relPath, err := filepath.Rel(kernel.SysFSRoot(), file)
		if err != nil {
			return err
		}

		value, err := readFileContent(file, ignoredBaseNames)
		if err != nil {
			return nil // Skip files that can't be read
		}

		s.InsertSnapshotEntry(s.Sys, relPath, string(value))
		return nil
	})

	return nil
}

// snapshotCPUFlags fetches the current CPU flags and adds them to the snapshot
func (s *Snapshot) snapshotCPUFlags() error {
	// no need for host proc path here, the cpuinfo file is always exposed
	info, err := cpu.Info()
	if err != nil {
		return err
	}
	if len(info) == 0 {
		return nil
	}

	s.CPUFlags = info[0].Flags
	slices.Sort(s.CPUFlags)
	return nil
}

// snapshotKernelCmdline fetches the current kernel command line parameters
func (s *Snapshot) snapshotKernelCmdline(ignoredBaseNames []string) error {
	// no need for the host proc path here, the cmdline file is always exposed
	value, err := readFileContent(kernel.HostProc("cmdline"), ignoredBaseNames)
	if err != nil {
		return err
	}
	s.KernelCmdline = string(value)
	return nil
}

// InsertSnapshotEntry inserts the provided file and its value in the input data
func (s *Snapshot) InsertSnapshotEntry(data map[string]interface{}, file string, value string) {
	if len(value) == 0 {
		// ignore
		return
	}

	parts := strings.Split(strings.TrimPrefix(file, "/"), string(os.PathSeparator))
	current := data
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = strings.TrimSpace(value)
		} else {
			if _, exists := current[part]; !exists {
				current[part] = make(map[string]interface{})
			}
			current = current[part].(map[string]interface{})
		}
	}
}

// ToJSON serializes the current Snapshot object to JSON
func (s *Snapshot) ToJSON() ([]byte, error) {
	return json.Marshal(s)
}

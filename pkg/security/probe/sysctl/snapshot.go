// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package sysctl is used to process and analyze sysctl data
package sysctl

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

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
func NewSnapshotEvent(ignoredBaseNames []string, kernelCompilationFlags map[string]uint8) (*SnapshotEvent, error) {
	se := &SnapshotEvent{
		Sysctl: NewSnapshot(),
	}
	if err := se.Sysctl.Snapshot(ignoredBaseNames, kernelCompilationFlags); err != nil {
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
	// KernelCompilationConfiguration contains the kernel compilation configuration
	KernelCompilationConfiguration map[string]string `json:"kernel_compilation_configuration,omitempty"`
}

// NewSnapshot returns a new sysctl snapshot
func NewSnapshot() Snapshot {
	return Snapshot{
		Proc:                           make(map[string]interface{}),
		Sys:                            make(map[string]interface{}),
		KernelCompilationConfiguration: make(map[string]string),
	}
}

// Snapshot runs the snapshot by going through the filesystem
func (s *Snapshot) Snapshot(ignoredBaseNames []string, kernelCompilationFlags map[string]uint8) error {
	if err := s.snapshotProcSys(ignoredBaseNames); err != nil {
		return fmt.Errorf("couldn't snapshot /proc/sys: %w", err)
	}

	if err := s.snapshotSys(ignoredBaseNames); err != nil {
		return fmt.Errorf("couldn't snapshot /sys: %w", err)
	}

	if err := s.snapshotCPUFlags(); err != nil {
		return fmt.Errorf("couldn't get CPU flags: %w", err)
	}

	if err := s.snapshotKernelCmdline(ignoredBaseNames); err != nil {
		return fmt.Errorf("couldn't get kernel cmdline: %w", err)
	}

	if err := s.snapshotKernelCompilationConfiguration(kernelCompilationFlags); err != nil {
		return fmt.Errorf("couldn't get kernel compilation configuration: %w", err)
	}
	return nil
}

// snapshotProcSys recursively reads files in /proc/sys and builds a nested JSON structure.
func (s *Snapshot) snapshotProcSys(ignoredBaseNames []string) error {
	return filepath.WalkDir(kernel.HostProc("/sys"), func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Skip if mode doesn't allow reading
		info, err := d.Info()
		if err != nil {
			return err
		}
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
	_ = filepath.WalkDir(kernel.HostSys("/firmware/efi/efivars/"), func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Skip if mode doesn't allow reading
		info, err := d.Info()
		if err != nil {
			return err
		}
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
	_ = filepath.WalkDir(kernel.HostSys("/devices/system/cpu/vulnerabilities"), func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Skip if mode doesn't allow reading
		info, err := d.Info()
		if err != nil {
			return err
		}
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
	flags, err := parseCPUFlags()
	if err != nil {
		return fmt.Errorf("error parsing CPU features/flags: %w", err)
	}
	slices.Sort(flags)
	s.CPUFlags = flags
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

func (s *Snapshot) getKernelConfigPath() (string, error) {
	kernelVersion, err := kernel.Release()
	if err != nil {
		return "", err
	}
	configPath := fmt.Sprintf(kernel.HostBoot("/config-%s"), strings.TrimSpace(string(kernelVersion)))
	if _, err := os.Stat(configPath); err == nil {
		return configPath, nil
	}
	procConfigGZ := kernel.HostProc("/config.gz")
	if _, err := os.Stat(procConfigGZ); err == nil {
		return procConfigGZ, nil
	}
	return "", errors.New("kernel config not found")
}

func (s *Snapshot) parseKernelConfig(r io.Reader, kernelCompilationFlags map[string]uint8) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			if strings.HasSuffix(line, "is not set") {
				key := string(bytes.Fields([]byte(line))[1])
				if _, ok := kernelCompilationFlags[key]; ok {
					s.KernelCompilationConfiguration[key] = "not_set"
				}
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok && kernelCompilationFlags[key] != 0 {
			s.KernelCompilationConfiguration[key] = strings.Trim(value, "\"")
		}
	}

	return scanner.Err()
}

// snapshotKernelCompilationConfiguration tries to resolve and parse the kernel compilation configuration
func (s *Snapshot) snapshotKernelCompilationConfiguration(kernelCompilationFlags map[string]uint8) error {
	configPath, err := s.getKernelConfigPath()
	if err != nil {
		return fmt.Errorf("error finding kernel config: %w", err)
	}

	var reader io.Reader
	file, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("error opening config file: %w", err)
	}
	defer file.Close()

	if strings.HasSuffix(configPath, ".gz") {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("error reading gzipped config: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	} else {
		reader = file
	}

	if err := s.parseKernelConfig(reader, kernelCompilationFlags); err != nil {
		return fmt.Errorf("error parsing kernel config: %w", err)
	}
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

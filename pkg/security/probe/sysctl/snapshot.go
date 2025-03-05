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

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var (
	redactedContent = "******"
)

// readFileContent reads a file and processes its content based on the given rules.
func readFileContent(file string, ignoredBaseNames []string) (string, error) {
	if slices.Contains(ignoredBaseNames, path.Base(file)) {
		return redactedContent, nil
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}

	return string(data), nil
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

		s.InsertSnapshotEntry(s.Proc, relPath, value)
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
		s.InsertSnapshotEntry(s.Sys, systemControl, value)
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

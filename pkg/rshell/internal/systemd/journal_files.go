// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxJournalFiles      = 4096
	maxMachineIDFileSize = 64
)

// journalFiles returns the machine ID and the paths of readable journal
// files. Only files ending in ".journal" are included: archived-corrupted
// files (".journal~") are never parsed for entries.
func (c *Client) journalFiles() (string, []string, error) {
	return c.journalFilesFiltered(false)
}

// journalAllocationFiles returns the machine ID and the paths of every file
// that counts toward reported journal disk usage, matching the file set
// collectVacuumCandidates sums allocated bytes over: readable ".journal"
// files plus recognized ".journal~" corruption archives.
func (c *Client) journalAllocationFiles() (string, []string, error) {
	return c.journalFilesFiltered(true)
}

func (c *Client) journalFilesFiltered(includeArchived bool) (string, []string, error) {
	if c.target.MachineIDPath == "" {
		return "", nil, fmt.Errorf("systemd target machine ID path is not configured")
	}
	if len(c.target.JournalDirs) == 0 {
		return "", nil, fmt.Errorf("systemd target journal directories are not configured")
	}

	machineID, err := c.readMachineID()
	if err != nil {
		return "", nil, err
	}

	files := make([]string, 0)
	for _, baseDir := range c.target.JournalDirs {
		dirPath := filepath.Join(baseDir, machineID)
		dir, err := c.openJournalMachineDirectory(dirPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", nil, fmt.Errorf("open journal directory %q: %w", dirPath, err)
		}

		entries, readErr := dir.ReadDir(maxJournalFiles + 1)
		closeErr := dir.Close()
		if readErr != nil && readErr != io.EOF {
			return "", nil, fmt.Errorf("read journal directory %q: %w", dirPath, readErr)
		}
		if closeErr != nil {
			return "", nil, fmt.Errorf("close journal directory %q: %w", dirPath, closeErr)
		}
		if len(entries) > maxJournalFiles {
			return "", nil, fmt.Errorf("journal directory %q has too many entries (maximum %d)", dirPath, maxJournalFiles)
		}

		for _, entry := range entries {
			if entry.Type()&fs.ModeSymlink != 0 {
				continue
			}
			matches := strings.HasSuffix(entry.Name(), ".journal")
			if includeArchived && !matches {
				matches = isArchivedJournalName(entry.Name())
			}
			if !matches {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return "", nil, fmt.Errorf("inspect journal file %q: %w", filepath.Join(dirPath, entry.Name()), err)
			}
			if !info.Mode().IsRegular() {
				continue
			}
			files = append(files, filepath.Join(dirPath, entry.Name()))
			if len(files) > maxJournalFiles {
				return "", nil, fmt.Errorf("systemd target has too many journal files (maximum %d)", maxJournalFiles)
			}
		}
	}

	sort.Strings(files)
	return machineID, files, nil
}

func (c *Client) openJournalMachineDirectory(path string) (*os.File, error) {
	before, err := c.lstatTargetPath(path)
	if err != nil {
		return nil, err
	}
	if !before.IsDir() || before.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("journal machine directory %q is not a real directory", path)
	}

	dir, err := c.openTargetFile(path)
	if err != nil {
		return nil, err
	}
	opened, err := dir.Stat()
	if err != nil {
		dir.Close()
		return nil, err
	}
	after, err := c.lstatTargetPath(path)
	if err != nil {
		dir.Close()
		return nil, err
	}
	if !opened.IsDir() || !after.IsDir() || after.Mode()&fs.ModeSymlink != 0 || !os.SameFile(before, opened) || !os.SameFile(after, opened) {
		dir.Close()
		return nil, fmt.Errorf("journal machine directory %q changed while opening", path)
	}
	return dir, nil
}

func readMachineID(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open systemd machine ID %q: %w", path, err)
	}
	return readMachineIDFile(path, file)
}

func (c *Client) readMachineID() (string, error) {
	file, err := c.openTargetFile(c.target.MachineIDPath)
	if err != nil {
		return "", fmt.Errorf("open systemd machine ID %q: %w", c.target.MachineIDPath, err)
	}
	return readMachineIDFile(c.target.MachineIDPath, file)
}

func readMachineIDFile(path string, file *os.File) (string, error) {
	data, readErr := io.ReadAll(io.LimitReader(file, maxMachineIDFileSize+1))
	closeErr := file.Close()
	if readErr != nil {
		return "", fmt.Errorf("read systemd machine ID %q: %w", path, readErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close systemd machine ID %q: %w", path, closeErr)
	}
	if len(data) > maxMachineIDFileSize {
		return "", fmt.Errorf("systemd machine ID file %q is too large", path)
	}

	machineID := strings.TrimSpace(string(data))
	if len(machineID) != 32 {
		return "", fmt.Errorf("systemd machine ID in %q must contain exactly 32 hexadecimal characters", path)
	}
	if !validID128(machineID) {
		return "", fmt.Errorf("systemd machine ID in %q contains a non-hexadecimal character", path)
	}
	return strings.ToLower(machineID), nil
}

func isArchivedJournalName(name string) bool {
	if strings.IndexByte(name, '/') >= 0 || strings.IndexByte(name, 0) >= 0 {
		return false
	}
	if strings.HasSuffix(name, ".journal~") {
		return validJournalArchiveStem(strings.TrimSuffix(name, ".journal~"), 2)
	}
	if !strings.HasSuffix(name, ".journal") {
		return false
	}
	return validJournalArchiveStem(strings.TrimSuffix(name, ".journal"), 3)
}

func validJournalArchiveStem(stem string, fields int) bool {
	separator := strings.LastIndexByte(stem, '@')
	if separator <= 0 || separator == len(stem)-1 {
		return false
	}
	parts := strings.Split(stem[separator+1:], "-")
	if len(parts) != fields {
		return false
	}
	if fields == 3 && !validHexLength(parts[0], 32) {
		return false
	}
	for _, part := range parts[fields-2:] {
		if !validHexLength(part, 16) {
			return false
		}
	}
	return true
}

func validHexLength(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

func validID128(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

const maxJournalQueryFiles = 128

var errJournalChanged = errors.New("systemd journal changed while being read")

type journalSnapshotFile struct {
	path string
	file *os.File
	info fs.FileInfo
	view *journalFileView
}

type journalSnapshot struct {
	machineID     journalID
	machineIDText string
	paths         []string
	files         []*journalSnapshotFile
}

func (c *Client) openJournalSnapshot() (*journalSnapshot, error) {
	machineIDText, paths, err := c.journalFiles()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, journalChanged("journal files changed during discovery")
		}
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no journal files found for machine %s", machineIDText)
	}
	if len(paths) > maxJournalQueryFiles {
		return nil, fmt.Errorf("%w: journal query opens %d files; maximum is %d", errJournalLimit, len(paths), maxJournalQueryFiles)
	}
	machineID, err := parseJournalID(machineIDText)
	if err != nil {
		return nil, err
	}

	snapshot := &journalSnapshot{
		machineID:     machineID,
		machineIDText: machineIDText,
		paths:         append([]string(nil), paths...),
		files:         make([]*journalSnapshotFile, 0, len(paths)),
	}
	fileIDs := make(map[journalID]string, len(paths))
	for _, path := range paths {
		opened, err := c.openJournalSnapshotFile(path)
		if err != nil {
			snapshot.close()
			return nil, err
		}
		if opened.view.header.machineID != machineID {
			opened.file.Close()
			snapshot.close()
			return nil, journalCorrupt(path, 40, "journal machine ID %s does not match configured machine %s", opened.view.header.machineID, machineID)
		}
		if opened.view.header.fileID.zero() {
			opened.file.Close()
			snapshot.close()
			return nil, journalCorrupt(path, 24, "journal file ID is zero")
		}
		if opened.view.header.seqnumID.zero() {
			opened.file.Close()
			snapshot.close()
			return nil, journalCorrupt(path, 72, "journal sequence ID is zero")
		}
		if previous, exists := fileIDs[opened.view.header.fileID]; exists {
			opened.file.Close()
			snapshot.close()
			return nil, journalCorrupt(path, 24, "journal file ID duplicates %q", previous)
		}
		fileIDs[opened.view.header.fileID] = path
		snapshot.files = append(snapshot.files, opened)
	}
	if err := snapshot.stable(c); err != nil {
		snapshot.close()
		return nil, err
	}
	return snapshot, nil
}

func (c *Client) openJournalSnapshotFile(path string) (*journalSnapshotFile, error) {
	before, err := c.lstatTargetPath(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, journalChanged("journal file %q disappeared before open", path)
		}
		return nil, fmt.Errorf("inspect journal file %q before open: %w", path, err)
	}
	if before.Mode()&fs.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return nil, journalChanged("journal file %q is no longer a regular non-symlink file", path)
	}

	file, err := c.openTargetFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, journalChanged("journal file %q disappeared during open", path)
		}
		return nil, fmt.Errorf("open journal file %q: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("inspect open journal file %q: %w", path, err)
	}
	after, err := c.lstatTargetPath(path)
	if err != nil {
		file.Close()
		if errors.Is(err, fs.ErrNotExist) {
			return nil, journalChanged("journal file %q disappeared during open", path)
		}
		return nil, fmt.Errorf("inspect journal file %q after open: %w", path, err)
	}
	if !info.Mode().IsRegular() || after.Mode()&fs.ModeSymlink != 0 || !after.Mode().IsRegular() || !os.SameFile(before, info) || !os.SameFile(after, info) {
		file.Close()
		return nil, journalChanged("journal file %q was replaced during open", path)
	}
	if info.Size() < 0 {
		file.Close()
		return nil, journalCorrupt(path, 0, "journal file has a negative size")
	}
	view, err := newJournalFileView(path, file, uint64(info.Size()))
	if err != nil {
		file.Close()
		return nil, err
	}
	return &journalSnapshotFile{path: path, file: file, info: info, view: view}, nil
}

func (s *journalSnapshot) stable(client *Client) error {
	machineID, paths, err := client.journalFiles()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return journalChanged("journal files changed during snapshot verification")
		}
		return err
	}
	if machineID != s.machineIDText || !sameJournalPaths(paths, s.paths) {
		return journalChanged("journal file set changed during snapshot")
	}
	for _, opened := range s.files {
		current, err := client.lstatTargetPath(opened.path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return journalChanged("journal file %q disappeared during snapshot", opened.path)
			}
			return fmt.Errorf("verify journal file %q: %w", opened.path, err)
		}
		if current.Mode()&fs.ModeSymlink != 0 || !current.Mode().IsRegular() || !os.SameFile(current, opened.info) {
			return journalChanged("journal file %q was replaced during snapshot", opened.path)
		}
		if err := opened.stable(); err != nil {
			return err
		}
	}
	return nil
}

func (f *journalSnapshotFile) stable() error {
	before, err := f.file.Stat()
	if err != nil {
		return fmt.Errorf("inspect open journal file %q before snapshot verification: %w", f.path, err)
	}
	if err := f.stableMetadata(before); err != nil {
		return err
	}

	header, err := readJournalHeader(f.path, f.file, uint64(before.Size()))
	if err != nil {
		if errors.Is(err, errJournalCorrupt) || errors.Is(err, errJournalUnsupported) {
			return journalChanged("journal file %q header changed during snapshot verification: %v", f.path, err)
		}
		return fmt.Errorf("verify journal file %q header: %w", f.path, err)
	}

	after, err := f.file.Stat()
	if err != nil {
		return fmt.Errorf("inspect open journal file %q after snapshot verification: %w", f.path, err)
	}
	if err := f.stableMetadata(after); err != nil {
		return err
	}
	if header != f.view.header {
		return journalChanged("journal file %q header changed during snapshot", f.path)
	}
	return nil
}

func (f *journalSnapshotFile) stableMetadata(current fs.FileInfo) error {
	if !current.Mode().IsRegular() || !os.SameFile(current, f.info) {
		return journalChanged("journal file %q was replaced during snapshot", f.path)
	}
	if current.Size() < f.info.Size() {
		return journalChanged("journal file %q shrank during snapshot", f.path)
	}
	if current.Size() > f.info.Size() {
		return journalChanged("journal file %q grew during snapshot", f.path)
	}
	if !current.ModTime().Equal(f.info.ModTime()) {
		return journalChanged("journal file %q modification time changed during snapshot", f.path)
	}
	return nil
}

func (s *journalSnapshot) close() error {
	var result error
	for _, opened := range s.files {
		if err := opened.file.Close(); err != nil {
			result = errors.Join(result, fmt.Errorf("close journal file %q: %w", opened.path, err))
		}
	}
	return result
}

func sameJournalPaths(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func parseJournalID(value string) (journalID, error) {
	var id journalID
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != len(id) {
		return journalID{}, fmt.Errorf("invalid 128-bit journal ID %q", value)
	}
	copy(id[:], decoded)
	return id, nil
}

func journalChanged(format string, arguments ...any) error {
	return fmt.Errorf("%w: %s", errJournalChanged, fmt.Sprintf(format, arguments...))
}

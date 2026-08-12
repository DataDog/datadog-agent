// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	sessionDirectoryPrefix = "datadog-authored-script-"
	workDirectoryName      = "work"
	homeDirectoryName      = "home"
	tempDirectoryName      = "tmp"
	sessionDirectoryMode   = 0700
)

// Session provides isolated writable directories for one script execution.
type Session struct {
	RootDirectory string
	WorkDirectory string
	HomeDirectory string
	TempDirectory string

	cleanupDirectory string
}

func NewSession() (*Session, error) {
	rootDirectory, err := os.MkdirTemp("", sessionDirectoryPrefix)
	if err != nil {
		return nil, fmt.Errorf("could not create authored-script session directory: %w", err)
	}

	created := false
	defer func() {
		if !created {
			_ = os.RemoveAll(rootDirectory)
		}
	}()

	session := &Session{
		RootDirectory:    rootDirectory,
		WorkDirectory:    filepath.Join(rootDirectory, workDirectoryName),
		HomeDirectory:    filepath.Join(rootDirectory, homeDirectoryName),
		TempDirectory:    filepath.Join(rootDirectory, tempDirectoryName),
		cleanupDirectory: rootDirectory,
	}
	for _, directory := range []string{session.WorkDirectory, session.HomeDirectory, session.TempDirectory} {
		if err := os.Mkdir(directory, sessionDirectoryMode); err != nil {
			return nil, fmt.Errorf("could not create authored-script session directory %q: %w", directory, err)
		}
	}

	created = true
	return session, nil
}

func (s *Session) Cleanup() error {
	if s.cleanupDirectory == "" {
		return nil
	}
	directory := s.cleanupDirectory
	if err := os.RemoveAll(directory); err != nil {
		return fmt.Errorf("could not remove authored-script session directory %q: %w", directory, err)
	}
	s.cleanupDirectory = ""
	return nil
}

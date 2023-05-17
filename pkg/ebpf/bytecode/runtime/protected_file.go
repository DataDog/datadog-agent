// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package runtime

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/justincormack/go-memfd"
)

// This represent a symlink to a sealed ram-backed file
type ProtectedFile interface {
	Close() error
	Reader() io.Reader
	Name() string
}

type ramBackedFile struct {
	symlink string
	file    *memfd.Memfd
}

// This function returns a sealed ram backed file
func NewProtectedFile(name, dir string, source io.Reader) (ProtectedFile, error) {
	var err error

	memfdFile, err := memfd.CreateNameFlags(name, memfd.AllowSealing|memfd.Cloexec)
	if err != nil {
		return nil, fmt.Errorf("failed to create memfd file: %w", err)
	}
	defer func() {
		if err != nil {
			memfdFile.Close()
		}
	}()

	if source != nil {
		if _, err = io.Copy(memfdFile, source); err != nil {
			return nil, fmt.Errorf("error copying bytes to memfd file: %w", err)
		}
	}

	// seal the memfd file, making it immutable.
	if err = memfdFile.SetSeals(memfd.SealAll); err != nil {
		return nil, fmt.Errorf("failed to seal memfd file: %w", err)
	}

	target := fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), memfdFile.Fd())
	tmpFile := filepath.Join(dir, name)

	os.Remove(tmpFile)
	if err := os.Symlink(target, tmpFile); err != nil {
		return nil, fmt.Errorf("failed to create symlink %s from target %s: %w", tmpFile, target, err)
	}

	if _, err := memfdFile.Seek(0, os.SEEK_SET); err != nil {
		return nil, fmt.Errorf("failed to reset cursor: %w", err)
	}

	return &ramBackedFile{
		file:    memfdFile,
		symlink: tmpFile,
	}, nil
}

// replace the symlink file with a copy of the input file so that
// debug info can be resolved by tools like objdump
func setupSourceInfoFile(source io.Reader, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, source); err != nil {
		return err
	}

	return nil
}

func (m *ramBackedFile) Close() error {
	os.Remove(m.symlink)
	if _, err := m.file.Seek(0, os.SEEK_SET); err != nil {
		log.Debug(err)
	}
	if err := setupSourceInfoFile(m.file, m.symlink); err != nil {
		log.Debugf("failed to setup source file: %v", err)
	}
	return m.file.Close()
}

func (m *ramBackedFile) Name() string {
	return m.symlink
}

func (m *ramBackedFile) Reader() io.Reader {
	return m.file
}

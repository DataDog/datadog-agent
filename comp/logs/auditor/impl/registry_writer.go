// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package auditorimpl

import (
	"os"
	"path/filepath"

	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
)

// atomicRegistryWriter implements atomic registry writing using a temporary file and rename
type atomicRegistryWriter struct{}

// NewAtomicRegistryWriter returns a new atomic registry writer
func NewAtomicRegistryWriter() auditor.RegistryWriter {
	return &atomicRegistryWriter{}
}

func (w *atomicRegistryWriter) WriteRegistry(registryPath string, registryDirPath string, registryTmpFile string, data []byte) error {
	f, err := os.CreateTemp(registryDirPath, registryTmpFile)
	if err != nil {
		return err
	}
	tmpName := f.Name()
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Chmod(0644); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, registryPath)
}

// nonAtomicRegistryWriter implements direct registry writing without atomic operations
type nonAtomicRegistryWriter struct{}

// NewNonAtomicRegistryWriter returns a new non-atomic registry writer
func NewNonAtomicRegistryWriter() auditor.RegistryWriter {
	return &nonAtomicRegistryWriter{}
}

func (w *nonAtomicRegistryWriter) WriteRegistry(registryPath string, _ string, _ string, data []byte) error {
	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(registryPath), 0755); err != nil {
		return err
	}

	// Write directly to the target file
	f, err := os.Create(registryPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Chmod(0644)
}

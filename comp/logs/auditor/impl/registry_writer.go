// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package auditorimpl

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"

	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
)

// registryWriteBufferSize is the size of the buffer between the registry
// encoder and the registry file.
const registryWriteBufferSize = 64 << 10

// encodeToFile runs encode against a buffered writer on f and flushes it.
func encodeToFile(f *os.File, encode func(io.Writer) error) error {
	w := bufio.NewWriterSize(f, registryWriteBufferSize)
	if err := encode(w); err != nil {
		return err
	}
	return w.Flush()
}

// atomicRegistryWriter implements atomic registry writing using a temporary file and rename
type atomicRegistryWriter struct{}

// NewAtomicRegistryWriter returns a new atomic registry writer
func NewAtomicRegistryWriter() auditor.RegistryWriter {
	return &atomicRegistryWriter{}
}

func (w *atomicRegistryWriter) WriteRegistry(registryPath string, registryDirPath string, registryTmpFile string, encode func(io.Writer) error) error {
	f, err := os.CreateTemp(registryDirPath, registryTmpFile)
	if err != nil {
		return err
	}
	tmpName := f.Name()
	defer func() {
		_ = f.Close()
		_ = os.Remove(tmpName)
	}()

	if err = encodeToFile(f, encode); err != nil {
		return err
	}
	if err = f.Chmod(0644); err != nil {
		return err
	}
	// Persist the complete file before making it visible at the registry path.
	// Atomic replacement alone does not guarantee that buffered file contents
	// survive an abrupt shutdown.
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return replaceRegistryFile(tmpName, registryPath)
}

// nonAtomicRegistryWriter implements direct registry writing without atomic operations
type nonAtomicRegistryWriter struct{}

// NewNonAtomicRegistryWriter returns a new non-atomic registry writer
func NewNonAtomicRegistryWriter() auditor.RegistryWriter {
	return &nonAtomicRegistryWriter{}
}

// WriteRegistry encodes the whole registry in memory before truncating the
// file, so the file is partial only for the length of a single write and an
// encoding error leaves it untouched.
func (w *nonAtomicRegistryWriter) WriteRegistry(registryPath string, _ string, _ string, encode func(io.Writer) error) error {
	var data bytes.Buffer
	if err := encode(&data); err != nil {
		return err
	}

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

	if _, err = f.Write(data.Bytes()); err != nil {
		return err
	}
	return f.Chmod(0644)
}

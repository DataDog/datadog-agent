// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package utils

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testLibsModule  = "test-module"
	testLibsProgram = "test-program"
	testLibsPID     = uint32(1234)
)

var (
	testLibsSSL    = PathIdentifier{Dev: 1, Inode: 1}
	testLibsCrypto = PathIdentifier{Dev: 1, Inode: 2}
)

// newLibsTestRegistry resets the debugger and registers an empty FileRegistry for
// testLibsProgram.
func newLibsTestRegistry(t *testing.T) *FileRegistry {
	ResetDebugger()
	t.Cleanup(ResetDebugger)
	r := &FileRegistry{
		byID:      make(map[PathIdentifier]*registration),
		byPID:     make(map[uint32]pathIdentifierSet),
		telemetry: registryTelemetry{programName: testLibsProgram},
	}
	debugger.AddRegistry(testLibsModule, r)
	return r
}

// registerLib records pid as traced through the library at path.
func registerLib(r *FileRegistry, pathID PathIdentifier, path string, pid uint32) {
	r.m.Lock()
	defer r.m.Unlock()
	r.byID[pathID] = &registration{sampleFilePath: path}
	if r.byPID[pid] == nil {
		r.byPID[pid] = make(pathIdentifierSet)
	}
	r.byPID[pid][pathID] = struct{}{}
}

func shortenLibrariesTracedTimeout(t *testing.T) {
	old := librariesTracedTimeout
	librariesTracedTimeout = 200 * time.Millisecond
	t.Cleanup(func() { librariesTracedTimeout = old })
}

func TestMissingTracedLibraries(t *testing.T) {
	r := newLibsTestRegistry(t)
	prefixes := []string{"libssl.so", "libcrypto.so"}

	assert.Equal(t, prefixes, missingTracedLibraries(testLibsModule, testLibsProgram, int(testLibsPID), prefixes))

	registerLib(r, testLibsSSL, "/usr/lib/libssl.so.3", testLibsPID)
	assert.Equal(t, []string{"libcrypto.so"}, missingTracedLibraries(testLibsModule, testLibsProgram, int(testLibsPID), prefixes))
	// Another pid is not traced through libssl.
	assert.Equal(t, prefixes, missingTracedLibraries(testLibsModule, testLibsProgram, int(testLibsPID)+1, prefixes))

	registerLib(r, testLibsCrypto, "/usr/lib/libcrypto.so.3", testLibsPID)
	assert.Empty(t, missingTracedLibraries(testLibsModule, testLibsProgram, int(testLibsPID), prefixes))
}

func TestWaitForLibrariesToBeTracedManualFallback(t *testing.T) {
	shortenLibrariesTracedTimeout(t)
	r := newLibsTestRegistry(t)
	registerLib(r, testLibsSSL, "/usr/lib/libssl.so.3", testLibsPID)

	// The manual attach fills in libcrypto but also reports an error for an unrelated
	// library. The helper must log that error and pass, since both libraries end up traced.
	attached := false
	AddAttacher(testLibsModule, testLibsProgram, &mockAttacher{overrideAttachPID: func(pid uint32) error {
		attached = true
		registerLib(r, testLibsCrypto, "/usr/lib/libcrypto.so.3", pid)
		return errors.New("partially hooked: unrelated library failed")
	}})

	WaitForLibrariesToBeTraced(t, testLibsModule, testLibsProgram, int(testLibsPID),
		[]string{"libssl.so", "libcrypto.so"}, ManualTracingFallbackEnabled)
	require.True(t, attached, "manual attach was not attempted")
}

func TestWaitForLibrariesToBeTracedAlreadyTraced(t *testing.T) {
	shortenLibrariesTracedTimeout(t)
	r := newLibsTestRegistry(t)
	registerLib(r, testLibsSSL, "/usr/lib/libssl.so.3", testLibsPID)
	registerLib(r, testLibsCrypto, "/usr/lib/libcrypto.so.3", testLibsPID)

	AddAttacher(testLibsModule, testLibsProgram, &mockAttacher{overrideAttachPID: func(uint32) error {
		t.Error("manual attach should not run when all libraries are traced")
		return nil
	}})

	WaitForLibrariesToBeTraced(t, testLibsModule, testLibsProgram, int(testLibsPID),
		[]string{"libssl.so", "libcrypto.so"}, ManualTracingFallbackEnabled)
}

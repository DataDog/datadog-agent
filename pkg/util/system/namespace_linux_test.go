// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNamespacesInodes(t *testing.T) {
	fakeProc := t.TempDir()

	assert.NoError(t, os.MkdirAll(filepath.Join(fakeProc, "1", "ns"), 0o755))
	assert.NoError(t, os.MkdirAll(filepath.Join(fakeProc, "2", "ns"), 0o755))

	assert.NoError(t, os.WriteFile(filepath.Join(fakeProc, "1", "ns", "net"), []byte{}, 0o644))
	assert.NoError(t, os.Link(filepath.Join(fakeProc, "1", "ns", "net"), filepath.Join(fakeProc, "2", "ns", "net")))

	_, err := GetProcessNamespaceInode(fakeProc, "2", "net")
	assert.NoError(t, err)
}

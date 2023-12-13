// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helpers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/flare/types"
)

type FlareBuilderMock struct {
	Fb   types.FlareBuilder
	Root string
	t    *testing.T
}

func NewFlareBuilderMock(t *testing.T, local bool) *FlareBuilderMock {
	root := t.TempDir()

	builder, err := newBuilder(root, "test-hostname", local)
	require.NoError(t, err)

	fb := &FlareBuilderMock{
		Fb:   builder,
		Root: builder.flareDir,
		t:    t,
	}
	t.Cleanup(func() { builder.logFile.Close() })
	return fb
}
func (m *FlareBuilderMock) filePath(path ...string) string {
	return filepath.Join(
		append(
			[]string{m.Root},
			path...,
		)...)
}

// AssertFileExists asserts that a file exists within the flare
func (m *FlareBuilderMock) AssertFileExists(paths ...string) bool {
	return assert.FileExists(m.t, m.filePath(paths...))
}

// AssertFileContent asserts that a file exists within the flare and has the correct content
func (m *FlareBuilderMock) AssertFileContent(content string, paths ...string) {
	path := m.filePath(paths...)

	if assert.FileExists(m.t, path) {
		data, err := os.ReadFile(path)
		require.NoError(m.t, err)
		assert.Equal(m.t, content, string(data), "Content of file %s is different from expected", path)
	}
}

// AssertFileContent asserts that a file exists within the flare and has the correct content
func (m *FlareBuilderMock) AssertFileContentMatch(pattern string, paths ...string) {
	path := m.filePath(paths...)

	if assert.FileExists(m.t, path) {
		data, err := os.ReadFile(path)
		require.NoError(m.t, err)
		assert.Regexp(m.t, pattern, string(data), "Content of file %s does not match Regexp", path)
	}
}

// AssertNoFileExists asserts that a file does not exists within the flare
func (m *FlareBuilderMock) AssertNoFileExists(paths ...string) bool {
	return assert.NoFileExists(m.t, m.filePath(paths...))
}

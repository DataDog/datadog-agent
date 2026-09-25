// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeODBCDrivers(t *testing.T, packagePath string) (msDriver, tdsDriver string) {
	t.Helper()
	msDriver = filepath.Join(packagePath, "embedded", "msodbcsql", "lib64", "libmsodbcsql-18.5.so.1.1")
	tdsDriver = filepath.Join(packagePath, "embedded", "lib", "libtdsodbc.so")
	for _, driver := range []string{msDriver, tdsDriver} {
		require.NoError(t, os.MkdirAll(filepath.Dir(driver), 0755))
		require.NoError(t, os.WriteFile(driver, nil, 0644))
	}
	return msDriver, tdsDriver
}

func TestEnsureODBCDriverConfigWritesDefault(t *testing.T) {
	packagePath := t.TempDir()
	msDriver, tdsDriver := writeODBCDrivers(t, packagePath)

	require.NoError(t, ensureODBCDriverConfig(packagePath))

	content, err := os.ReadFile(filepath.Join(packagePath, "embedded", "etc", "odbcinst.ini"))
	require.NoError(t, err)
	assert.Equal(t, defaultODBCInst(msDriver, tdsDriver), string(content))
}

func TestEnsureODBCDriverConfigResolvesSymlinkedPackagePath(t *testing.T) {
	realPath := t.TempDir()
	msDriver, tdsDriver := writeODBCDrivers(t, realPath)
	linkPath := filepath.Join(t.TempDir(), "stable")
	require.NoError(t, os.Symlink(realPath, linkPath))

	require.NoError(t, ensureODBCDriverConfig(linkPath))

	content, err := os.ReadFile(filepath.Join(realPath, "embedded", "etc", "odbcinst.ini"))
	require.NoError(t, err)
	assert.Equal(t, defaultODBCInst(msDriver, tdsDriver), string(content))
}

func TestRewriteODBCInst(t *testing.T) {
	const (
		pkg       = "/opt/datadog-packages/datadog-agent/7.70.0"
		msDriver  = pkg + "/embedded/msodbcsql/lib64/libmsodbcsql-18.5.so.1.1"
		tdsDriver = pkg + "/embedded/lib/libtdsodbc.so"
		pinned    = pkg + "/embedded/msodbcsql/lib64/libmsodbcsql-18.3.so.3.1"
	)
	exists := func(path string) bool { return path == pinned }

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "rewrites deb install path",
			content:  "[FreeTDS]\nDriver=/opt/datadog-agent/embedded/lib/libtdsodbc.so\n",
			expected: "[FreeTDS]\nDriver=" + tdsDriver + "\n",
		},
		{
			name:     "rewrites previous package version",
			content:  "[ODBC Driver 18 for SQL Server]\nDriver=/opt/datadog-packages/datadog-agent/7.64.0/embedded/msodbcsql/lib64/libmsodbcsql-18.3.so.3.1\n",
			expected: "[ODBC Driver 18 for SQL Server]\nDriver=" + msDriver + "\n",
		},
		{
			name:     "keeps host driver",
			content:  "[ODBC Driver 18 for SQL Server]\nDriver=/opt/microsoft/msodbcsql18/lib64/libmsodbcsql-18.6.so.1.1\n",
			expected: "[ODBC Driver 18 for SQL Server]\nDriver=/opt/microsoft/msodbcsql18/lib64/libmsodbcsql-18.6.so.1.1\n",
		},
		{
			name:     "keeps existing library inside package",
			content:  "[ODBC Driver 18 for SQL Server]\nDriver=" + pinned + "\n",
			expected: "[ODBC Driver 18 for SQL Server]\nDriver=" + pinned + "\n",
		},
		{
			name:     "keeps unknown section",
			content:  "[Other]\nDriver=/opt/datadog-agent/embedded/lib/libother.so\n",
			expected: "[Other]\nDriver=/opt/datadog-agent/embedded/lib/libother.so\n",
		},
		{
			name:     "matches section case-insensitively",
			content:  "[freetds]\nDriver=/opt/datadog-agent/embedded/lib/libtdsodbc.so\n",
			expected: "[freetds]\nDriver=" + tdsDriver + "\n",
		},
		{
			name:     "preserves CRLF",
			content:  "[FreeTDS]\r\nDriver=/opt/datadog-agent/embedded/lib/libtdsodbc.so\r\nUsageCount=1\r\n",
			expected: "[FreeTDS]\r\nDriver=" + tdsDriver + "\r\nUsageCount=1\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, rewriteODBCInst(tt.content, pkg, msDriver, tdsDriver, exists))
		})
	}
}

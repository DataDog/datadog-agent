// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package integrations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultDatabaseFromURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want string
	}{
		{
			name: "database in path",
			uri:  "postgres://user:pass@localhost:5432/mydb",
			want: "mydb",
		},
		{
			name: "no path falls back to postgres",
			uri:  "postgres://user:pass@localhost:5432",
			want: "postgres",
		},
		{
			name: "root path falls back to postgres",
			uri:  "postgres://user:pass@localhost:5432/",
			want: "postgres",
		},
		{
			name: "slash in sslrootcert query param is not mistaken for the database",
			uri:  "postgres://user:pass@localhost:5432/mydb?sslrootcert=/etc/ssl/ca.pem",
			want: "mydb",
		},
		{
			name: "slash in unix-socket host query param is not mistaken for the database",
			uri:  "postgres://user:pass@localhost:5432/mydb?host=/var/run/postgresql",
			want: "mydb",
		},
		{
			name: "unparseable URI falls back to postgres",
			uri:  "not a valid uri :::",
			want: "postgres",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, defaultDatabaseFromURI(tt.uri))
		})
	}
}

// fakePython writes an executable script standing in for the Python
// interpreter so callPython can be exercised without a real embedded Python
// environment or the integrations-core setup module.
func fakePython(t *testing.T, stdout string, exitCode int) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "fake-python.sh")
	script := fmt.Sprintf("#!/bin/sh\ncat <<'EOF'\n%s\nEOF\nexit %d\n", stdout, exitCode)
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

func TestCallPython(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake python stand-in is a shell script")
	}

	ctx := context.Background()
	params := &setupParams{datadogUser: "datadog"}

	t.Run("success envelope returns result", func(t *testing.T) {
		pythonPath := fakePython(t, `{"success":true,"result":{"flavor":"self_hosted","outcome":"success"}}`, 0)

		result, err := callPython(ctx, pythonPath, "postgres://localhost/mydb", []string{"mydb"}, params, false)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "success", result.Outcome)
	})

	t.Run("structured failure envelope surfaces the script's error message", func(t *testing.T) {
		pythonPath := fakePython(t, `{"success":false,"error":"could not connect: ACCESS DENIED"}`, 1)

		result, err := callPython(ctx, pythonPath, "postgres://localhost/mydb", []string{"mydb"}, params, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ACCESS DENIED")
		assert.Nil(t, result)
	})

	t.Run("non-JSON output with a failing exit code reports the run error", func(t *testing.T) {
		pythonPath := fakePython(t, `Traceback (most recent call last): boom`, 1)

		result, err := callPython(ctx, pythonPath, "postgres://localhost/mydb", []string{"mydb"}, params, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "setup script failed")
		assert.Nil(t, result)
	})

	t.Run("non-JSON output with a successful exit code reports a parse error", func(t *testing.T) {
		pythonPath := fakePython(t, `not json at all`, 0)

		result, err := callPython(ctx, pythonPath, "postgres://localhost/mydb", []string{"mydb"}, params, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parsing setup output")
		assert.Nil(t, result)
	})
}

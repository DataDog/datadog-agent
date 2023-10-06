// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package helpers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createPermsTestFile(t *testing.T) (string, string, string) {
	tempDir := t.TempDir()

	f1 := filepath.Join(tempDir, "file1")
	f2 := filepath.Join(tempDir, "file2")
	f3 := filepath.Join(tempDir, "file3")

	os.WriteFile(f1, nil, 0666)
	os.WriteFile(f2, nil, 0444)

	return f1, f2, f3
}

func TestPermsFileAdd(t *testing.T) {
	f1, f2, f3 := createPermsTestFile(t)

	pi := permissionsInfos{}
	pi.add(f1)
	pi.add(f2)
	pi.add(f3)

	require.Len(t, pi, 3)

	assert.Equal(t, f1, pi[f1].path)
	assert.Equal(t, "-rw-rw-rw-", pi[f1].mode)
	assert.NotNil(t, pi[f1].acls)
	assert.LessOrEqual(t, 1, len(pi[f1].acls))
	assert.NotEmpty(t, pi[f1].acls[0].userName)
	assert.NotEmpty(t, pi[f1].acls[0].accessMask)

	assert.Equal(t, f2, pi[f2].path)
	assert.Equal(t, "-r--r--r--", pi[f2].mode)
	assert.NotNil(t, pi[f2].acls)
	assert.LessOrEqual(t, 1, len(pi[f2].acls))
	assert.NotEmpty(t, pi[f2].acls[0].userName)
	assert.NotEmpty(t, pi[f2].acls[0].accessMask)

	assert.Equal(t, f3, pi[f3].path)
	assert.Empty(t, pi[f3].mode)
	assert.NotNil(t, pi[f3].err)
	assert.Nil(t, pi[f3].acls)
}

func TestPermsFileCommit(t *testing.T) {
	f1, f2, f3 := createPermsTestFile(t)

	pi := permissionsInfos{}
	pi.add(f1)
	pi.add(f2)
	pi.add(f3)

	// No error
	b, err := pi.commit()
	require.NoError(t, err)

	// file1 and file2 header
	assert.Regexp(t, "(?ms)^File: C:\\\\.+\\\\file1$", string(b))
	assert.Regexp(t, "(?ms)^File: C:\\\\.+\\\\file2$", string(b))
	assert.Regexp(t, "(?ms)^File: C:\\\\.+\\\\file3$", string(b))

	// check no error was raised for file21 and file2
	assert.NotRegexp(t, "(?ms)^could not stat file: C:\\\\.+\\\\file1", string(b))
	assert.NotRegexp(t, "(?ms)^could not stat file: C:\\\\.+\\\\file2", string(b))

	// check that an error was raised for file3
	assert.Regexp(t, "(?ms)^could not stat file.+C:\\\\.+\\\\file3", string(b))
}

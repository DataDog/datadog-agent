// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package helpers

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createPermsTestFile(t *testing.T) (string, string, string) {
	tempDir := t.TempDir()

	f1 := filepath.Join(tempDir, "file_1")
	f2 := filepath.Join(tempDir, "file_2")
	f3 := filepath.Join(tempDir, "file_3")

	// Because of umask the rights for newly created file might not be the one we asked for. We enforce it with
	// os.Chmod.
	os.WriteFile(f1, nil, 0765)
	os.Chmod(f1, 0765)
	os.WriteFile(f2, nil, 0400)
	os.Chmod(f2, 0400)
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
	assert.Equal(t, "-rwxrw-r-x", pi[f1].mode)
	assert.NotNil(t, pi[f1].owner)
	assert.NotNil(t, pi[f1].group)
	assert.Nil(t, pi[f1].err)

	assert.Equal(t, f2, pi[f2].path)
	assert.Equal(t, "-r--------", pi[f2].mode)
	assert.NotNil(t, pi[f2].owner)
	assert.NotNil(t, pi[f2].group)
	assert.Nil(t, pi[f2].err)

	assert.Equal(t, f2, pi[f2].path)
	assert.Equal(t, "-r--------", pi[f2].mode)
	assert.NotNil(t, pi[f2].owner)
	assert.NotNil(t, pi[f2].group)
	assert.Nil(t, pi[f2].err)

	assert.Equal(t, f3, pi[f3].path)
	assert.Empty(t, pi[f3].mode)
	assert.Empty(t, pi[f3].owner)
	assert.Empty(t, pi[f3].group)
	assert.NotNil(t, pi[f3].err)
}

func TestPermsFileCommit(t *testing.T) {
	f1, f2, f3 := createPermsTestFile(t)

	pi := permissionsInfos{}
	pi.add(f1)
	pi.add(f2)
	pi.add(f3)

	b, err := pi.commit()
	require.NoError(t, err)

	pattern := []byte(`File path                                          | mode  | owner      | group      | error     |
---------------------------------------------------------------------------------------------------
.+/file_a      | -rwxrw---- | \w+\s*| \w+\s*|           |
.+/file_a      | -r-------- | \w+\s*| \w+\s*|           |
.+/file_a      |       |            |            | could not stat file .+/file_3: stat .+/file_3: no such file or directory|`)

	res, _ := regexp.Match(string(b), pattern)
	assert.True(t, res)
}

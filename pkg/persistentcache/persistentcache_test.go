// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

package persistentcache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePersistentCache(t *testing.T) {
	testDir := t.TempDir()
	mockConfig := config.Mock(t)
	mockConfig.SetWithoutSource("run_path", testDir)
	err := Write("mykey", "myvalue")
	assert.Nil(t, err)
	value, err := Read("mykey")
	assert.Equal(t, "myvalue", value)
	assert.Nil(t, err)
	value, err = Read("myotherkey")
	assert.Equal(t, "", value)
	assert.Nil(t, err)
}

func TestWritePersistentCacheColons(t *testing.T) {
	testDir := t.TempDir()
	mockConfig := config.Mock(t)
	mockConfig.SetWithoutSource("run_path", testDir)
	err := Write("my:key", "myvalue")
	assert.Nil(t, err)
	value, err := Read("my:key")
	assert.Equal(t, "myvalue", value)
	assert.Nil(t, err)

	expectPath := filepath.Join(testDir, "my")
	_, err = os.Stat(expectPath)
	require.Nil(t, err)

	expectPathFile := filepath.Join(testDir, "my", "key")
	_, err = os.Stat(expectPathFile)
	require.Nil(t, err)
}

func TestWritePersistentCacheInvalidChar(t *testing.T) {
	testDir := t.TempDir()
	mockConfig := config.Mock(t)
	mockConfig.SetWithoutSource("run_path", testDir)
	err := Write("my/key", "myvalue")
	assert.Nil(t, err)
	value, err := Read("my/key")
	assert.Equal(t, "myvalue", value)
	assert.Nil(t, err)

	expectPathFile := filepath.Join(testDir, "mykey")
	_, err = os.Stat(expectPathFile)
	require.Nil(t, err)

	err = Write("my:key*foo", "myvalue")
	assert.Nil(t, err)
	expectPathFile = filepath.Join(testDir, "my", "keyfoo")
	_, err = os.Stat(expectPathFile)
	require.Nil(t, err)

	err = Write("my\\key", "myvalue")
	assert.Nil(t, err)
	expectPathFile = filepath.Join(testDir, "mykey")
	_, err = os.Stat(expectPathFile)
	require.Nil(t, err)

	err = Write("my|di*r:key<foo", "myvalue")
	assert.Nil(t, err)
	expectPathFile = filepath.Join(testDir, "mydir", "keyfoo")
	_, err = os.Stat(expectPathFile)
	require.Nil(t, err)

	err = Write("key_foo-bar", "myvalue")
	assert.Nil(t, err)
	expectPathFile = filepath.Join(testDir, "key_foo-bar")
	_, err = os.Stat(expectPathFile)
	require.Nil(t, err)
}

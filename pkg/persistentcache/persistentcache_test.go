// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

package persistentcache

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/config"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePersistentCache(t *testing.T) {
	testDir, err := ioutil.TempDir("", "fake-datadog-run-")
	require.Nil(t, err, fmt.Sprintf("%v", err))
	defer os.RemoveAll(testDir)
	mockConfig := config.Mock()
	mockConfig.Set("run_path", testDir)
	err = Write("mykey", "myvalue")
	assert.Nil(t, err)
	value, err := Read("mykey")
	assert.Equal(t, "myvalue", value)
	assert.Nil(t, err)
	value, err = Read("myotherkey")
	assert.Equal(t, "", value)
	assert.Nil(t, err)
}

func TestWritePersistentCacheInvalidChar(t *testing.T) {
	testDir, err := ioutil.TempDir("", "fake-datadog-run-")
	require.Nil(t, err, fmt.Sprintf("%v", err))
	defer os.RemoveAll(testDir)
	mockConfig := config.Mock()
	mockConfig.Set("run_path", testDir)
	err = Write("my:key", "myvalue")
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

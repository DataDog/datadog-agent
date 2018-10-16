// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package security

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestFetchAuthTokenValidGen(t *testing.T) {
	testDir, err := ioutil.TempDir("", "fake-datadog-etc-")
	require.Nil(t, err, fmt.Sprintf("%v", err))
	defer os.RemoveAll(testDir)

	f, err := ioutil.TempFile(testDir, "fake-datadog-yaml-")
	require.Nil(t, err, fmt.Errorf("%v", err))
	defer os.Remove(f.Name())

	mockConfig := config.NewMock()
	mockConfig.SetConfigFile(f.Name())
	expectTokenPath := filepath.Join(testDir, "auth_token")
	defer os.Remove(expectTokenPath)

	mockConfig.Set("auth_token", "")
	token, err := FetchAuthToken()
	require.Nil(t, err, fmt.Sprintf("%v", err))
	assert.True(t, len(token) > authTokenMinimalLen, fmt.Sprintf("%d", len(token)))
	_, err = os.Stat(expectTokenPath)
	require.Nil(t, err)
}

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
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestFetchAuthTokenTooShort(t *testing.T) {
	config.Datadog.Set("auth_token", "123")
	token, err := FetchAuthToken()
	require.NotNil(t, err, fmt.Sprintf("%v", err))
	assert.Equal(t, err, fmt.Errorf("invalid authentication token, length is 3: must be at least %d characters", authTokenLen))
	assert.Equal(t, "", token)
}

func TestFetchAuthTokenValidConf(t *testing.T) {
	const expectedToken = "1234567890123456789012345678901234567890"
	config.Datadog.Set("auth_token", expectedToken)
	token, err := FetchAuthToken()
	require.Nil(t, err, fmt.Sprintf("%v", err))
	assert.Equal(t, expectedToken, token)
}

func TestFetchAuthTokenValidGen(t *testing.T) {
	testDir, err := ioutil.TempDir("", "fake-datadog-etc-")
	require.Nil(t, err, fmt.Sprintf("%v", err))
	defer os.RemoveAll(testDir)

	f, err := ioutil.TempFile(testDir, "fake-datadog-yaml-")
	require.Nil(t, err, fmt.Errorf("%v", err))
	defer os.Remove(f.Name())

	config.Datadog.SetConfigFile(f.Name())
	expectTokenPath := path.Join(testDir, "auth_token")
	defer os.Remove(expectTokenPath)

	config.Datadog.Set("auth_token", "")
	token, err := FetchAuthToken()
	require.Nil(t, err, fmt.Sprintf("%v", err))
	assert.True(t, len(token) > authTokenLen, fmt.Sprintf("%d", len(token)))
	_, err = os.Stat(expectTokenPath)
	require.Nil(t, err)
}

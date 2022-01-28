// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !serverless

package util

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

func TestGetHostnameFromHostnameConfig(t *testing.T) {
	clearCache()
	config.Datadog.Set("hostname", "expectedhostname")
	config.Datadog.Set("hostname_file", "")
	defer cleanUpConfigValues()

	hostname, err := GetHostname(context.TODO())
	if !assert.Nil(t, err) {
		return
	}

	assert.Equal(t, "expectedhostname", hostname)
}

func TestGetHostnameCaching(t *testing.T) {
	clearCache()
	config.Datadog.Set("hostname", "expectedhostname")
	config.Datadog.Set("hostname_file", "badhostname")
	defer cleanUpConfigValues()

	hostname, err := GetHostname(context.TODO())
	if !assert.Nil(t, err) {
		return
	}
	assert.Equal(t, "expectedhostname", hostname)

	config.Datadog.Set("hostname", "newhostname")
	hostname, err = GetHostname(context.TODO())
	if !assert.Nil(t, err) {
		return
	}
	assert.Equal(t, "expectedhostname", hostname)
}

func TestGetHostnameFromHostnameFileConfig(t *testing.T) {
	hostnameFile, err := writeTempHostnameFile("expectedfilehostname")
	if !assert.Nil(t, err) {
		return
	}
	defer os.RemoveAll(hostnameFile)

	config.Datadog.Set("hostname", "")
	config.Datadog.Set("hostname_file", hostnameFile)
	defer cleanUpConfigValues()

	hostname, err := GetHostname(context.TODO())
	if !assert.Nil(t, err) {
		return
	}

	assert.Equal(t, "expectedfilehostname", hostname)
}

func TestNormalizeHost(t *testing.T) {
	assert := assert.New(t)
	long := strings.Repeat("a", 256)
	assert.Len(NormalizeHost(long), 0, "long host should be dropped")
	assert.Equal("a-b", NormalizeHost("a<b"), "< defanged")
	assert.Equal("a-b", NormalizeHost("a>b"), "> defanged")
	assert.Equal("example.com", NormalizeHost("example.com\r\n"), "NL/CR dropped")

	// Invalid host name as bytes that would look like this: 9cbef2d1-8c20-4bf2-97a5-7d70��
	b := []byte{
		57, 99, 98, 101, 102, 50, 100, 49, 45, 56, 99, 50, 48, 45,
		52, 98, 102, 50, 45, 57, 55, 97, 53, 45, 55, 100, 55, 48,
		0, 0, 0, 0, 239, 191, 189, 239, 191, 189, 1, // these are bad bytes
	}
	assert.Equal("", NormalizeHost(string(b)))
}

func writeTempHostnameFile(content string) (string, error) {
	destFile, err := ioutil.TempFile("", "test-hostname-file-config-")
	if err != nil {
		return "", err
	}

	err = ioutil.WriteFile(destFile.Name(), []byte(content), os.ModePerm)
	if err != nil {
		os.RemoveAll(destFile.Name())
		return "", err
	}

	return destFile.Name(), nil
}

func cleanUpConfigValues() {
	clearCache()
	config.Datadog.Set("hostname", "")
	config.Datadog.Set("hostname_file", "")
}

func clearCache() {
	cache.Cache.Flush()
}

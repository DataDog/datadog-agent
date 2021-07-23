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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"os"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func setupTest() {
	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	config.InitConfig(config.Datadog)
	os.Setenv("AWS_LAMBDA_FUNCTION_NAME", "TestFunction")
}

func TestProxyNotLoaded(t *testing.T) {
	setupTest()

	proxyHTTP := "a:1"
	proxyHTTPS := "a:2"
	t.Setenv("DD_PROXY_HTTP", proxyHTTP)
	t.Setenv("DD_PROXY_HTTPS", proxyHTTPS)
	proxyHTTPConfig := config.Datadog.GetString("proxy.http")
	proxyHTTPSConfig := config.Datadog.GetString("proxy.https")
	assert.Equal(t, 0, len(proxyHTTPConfig))
	assert.Equal(t, 0, len(proxyHTTPSConfig))
}

func TestProxyLoadedFromEnvVars(t *testing.T) {
	setupTest()

	proxyHTTP := "b:1"
	proxyHTTPS := "b:2"
	t.Setenv("DD_PROXY_HTTP", proxyHTTP)
	t.Setenv("DD_PROXY_HTTPS", proxyHTTPS)

	config.LoadWithoutSecret()
	proxyHTTPConfig := config.Datadog.GetString("proxy.http")
	proxyHTTPSConfig := config.Datadog.GetString("proxy.https")

	assert.Equal(t, proxyHTTP, proxyHTTPConfig)
	assert.Equal(t, proxyHTTPS, proxyHTTPSConfig)
}

func TestProxyLoadedFromConfigFile(t *testing.T) {
	setupTest()

	tempDir := t.TempDir()
	configTest := path.Join(tempDir, "datadog.yaml")
	os.WriteFile(configTest, []byte("proxy:\n  http: \"c:1\"\n  https: \"c:2\""), 0644)

	config.Datadog.AddConfigPath(tempDir)
	config.LoadWithoutSecret()
	proxyHTTPConfig := config.Datadog.GetString("proxy.http")
	proxyHTTPSConfig := config.Datadog.GetString("proxy.https")

	assert.Equal(t, "c:1", proxyHTTPConfig)
	assert.Equal(t, "c:2", proxyHTTPSConfig)
}

func TestProxyLoadedFromConfigFileAndEnvVars(t *testing.T) {
	setupTest()

	proxyHTTPEnvVar := "d:1"
	proxyHTTPSEnvVar := "d:2"
	t.Setenv("DD_PROXY_HTTP", proxyHTTPEnvVar)
	t.Setenv("DD_PROXY_HTTPS", proxyHTTPSEnvVar)

	tempDir := t.TempDir()
	configTest := path.Join(tempDir, "datadog.yaml")
	os.WriteFile(configTest, []byte("proxy:\n  http: \"e:1\"\n  https: \"e:2\""), 0644)

	config.Datadog.AddConfigPath(tempDir)
	config.LoadWithoutSecret()
	proxyHTTPConfig := config.Datadog.GetString("proxy.http")
	proxyHTTPSConfig := config.Datadog.GetString("proxy.https")

	assert.Equal(t, proxyHTTPEnvVar, proxyHTTPConfig)
	assert.Equal(t, proxyHTTPSEnvVar, proxyHTTPSConfig)
}

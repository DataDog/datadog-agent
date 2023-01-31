// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package main

import (
	"testing"
	"github.com/DataDog/datadog-agent/pkg/config"
	"gotest.tools/assert"
)

func TestProxyNotLoaded(t *testing.T) {
	proxyHttp := "abc:1234"
	proxyHttps := "abc:5678"
	t.Setenv("DD_PROXY_HTTP", proxyHttp)
	t.Setenv("DD_PROXY_HTTPS", proxyHttps)
	proxyHttpConfig := config.Datadog.GetString("proxy.http")
	proxyHttpsConfig := config.Datadog.GetString("proxy.https")
	assert.Equal(t, 0, len(proxyHttpConfig))
	assert.Equal(t, 0, len(proxyHttpsConfig))
}

func TestProxyLoaded(t *testing.T) {
	proxyHttp := "abc:1234"
	proxyHttps := "abc:5678"
	t.Setenv("DD_PROXY_HTTP", proxyHttp)
	t.Setenv("DD_PROXY_HTTPS", proxyHttps)
	setupProxy()
	proxyHttpConfig := config.Datadog.GetString("proxy.http")
	proxyHttpsConfig := config.Datadog.GetString("proxy.https")
	assert.Equal(t, proxyHttp, proxyHttpConfig)
	assert.Equal(t, proxyHttps, proxyHttpsConfig)
}

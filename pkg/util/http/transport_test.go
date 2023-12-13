// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"testing"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmptyProxy(t *testing.T) {
	setupTest(t)

	r, err := http.NewRequest("GET", "https://test.com", nil)
	require.Nil(t, err)

	proxies := &pkgconfigmodel.Proxy{}
	c := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	proxyFunc := GetProxyTransportFunc(proxies, c)

	proxyURL, err := proxyFunc(r)
	assert.Nil(t, err)
	assert.Nil(t, proxyURL)
}

func TestHTTPProxy(t *testing.T) {
	setupTest(t)

	rHTTP, _ := http.NewRequest("GET", "http://test.com/api/v1?arg=21", nil)
	rHTTPS, _ := http.NewRequest("GET", "https://test.com/api/v1?arg=21", nil)

	proxies := &pkgconfigmodel.Proxy{
		HTTP: "https://user:pass@proxy.com:3128",
	}
	c := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	proxyFunc := GetProxyTransportFunc(proxies, c)

	proxyURL, err := proxyFunc(rHTTP)
	assert.Nil(t, err)
	assert.Equal(t, "https://user:pass@proxy.com:3128", proxyURL.String())

	proxyURL, err = proxyFunc(rHTTPS)
	assert.Nil(t, err)
	assert.Nil(t, proxyURL)
}

func TestNoProxy(t *testing.T) {
	setupTest(t)

	r1, _ := http.NewRequest("GET", "http://test_no_proxy.com/api/v1?arg=21", nil)
	r2, _ := http.NewRequest("GET", "http://test_http.com/api/v1?arg=21", nil)
	r3, _ := http.NewRequest("GET", "https://test_https.com/api/v1?arg=21", nil)
	r4, _ := http.NewRequest("GET", "http://sub.test_no_proxy.com/api/v1?arg=21", nil)

	proxies := &pkgconfigmodel.Proxy{
		HTTP:    "https://user:pass@proxy.com:3128",
		HTTPS:   "https://user:pass@proxy_https.com:3128",
		NoProxy: []string{"test_no_proxy.com", "test.org"},
	}
	c := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))

	proxyFunc := GetProxyTransportFunc(proxies, c)

	proxyURL, err := proxyFunc(r1)
	assert.Nil(t, err)
	assert.Nil(t, proxyURL)

	proxyURL, err = proxyFunc(r2)
	assert.Nil(t, err)
	assert.Equal(t, "https://user:pass@proxy.com:3128", proxyURL.String())

	proxyURL, err = proxyFunc(r3)
	assert.Nil(t, err)
	assert.Equal(t, "https://user:pass@proxy_https.com:3128", proxyURL.String())

	// Validate the old behavior (when no_proxy_nonexact_match is false)
	proxyURL, err = proxyFunc(r4)
	assert.Nil(t, err)
	assert.Equal(t, "https://user:pass@proxy.com:3128", proxyURL.String())
}

func TestNoProxyNonexactMatch(t *testing.T) {
	setupTest(t)

	r1, _ := http.NewRequest("GET", "http://test_no_proxy.com/api/v1?arg=21", nil)
	r2, _ := http.NewRequest("GET", "http://test_http.com/api/v1?arg=21", nil)
	r3, _ := http.NewRequest("GET", "https://test_https.com/api/v1?arg=21", nil)
	r4, _ := http.NewRequest("GET", "http://sub.test_no_proxy.com/api/v1?arg=21", nil)
	r5, _ := http.NewRequest("GET", "http://no_proxy2.com/api/v1?arg=21", nil)
	r6, _ := http.NewRequest("GET", "http://sub.no_proxy2.com/api/v1?arg=21", nil)

	c := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	c.SetWithoutSource("no_proxy_nonexact_match", true)

	// Testing some nonexact matching cases as documented here: https://github.com/golang/net/blob/master/http/httpproxy/proxy.go#L38
	proxies := &pkgconfigmodel.Proxy{
		HTTP:    "https://user:pass@proxy.com:3128",
		HTTPS:   "https://user:pass@proxy_https.com:3128",
		NoProxy: []string{"test_no_proxy.com", "test.org", ".no_proxy2.com"},
	}
	proxyFunc := GetProxyTransportFunc(proxies, c)

	proxyURL, err := proxyFunc(r1)
	assert.Nil(t, err)
	assert.Nil(t, proxyURL)

	proxyURL, err = proxyFunc(r2)
	assert.Nil(t, err)
	assert.Equal(t, "https://user:pass@proxy.com:3128", proxyURL.String())

	proxyURL, err = proxyFunc(r3)
	assert.Nil(t, err)
	assert.Equal(t, "https://user:pass@proxy_https.com:3128", proxyURL.String())

	proxyURL, err = proxyFunc(r4)
	assert.Nil(t, err)
	assert.Nil(t, proxyURL)

	proxyURL, err = proxyFunc(r5)
	assert.Nil(t, err)
	assert.Equal(t, "https://user:pass@proxy.com:3128", proxyURL.String())

	proxyURL, err = proxyFunc(r6)
	assert.Nil(t, err)
	assert.Nil(t, proxyURL)
}

func TestErrorParse(t *testing.T) {
	setupTest(t)

	r1, _ := http.NewRequest("GET", "http://test_no_proxy.com/api/v1?arg=21", nil)

	proxies := &pkgconfigmodel.Proxy{
		HTTP: "21://test.com",
	}
	c := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	proxyFunc := GetProxyTransportFunc(proxies, c)

	proxyURL, err := proxyFunc(r1)
	assert.NotNil(t, err)
	assert.Nil(t, proxyURL)
}

func TestBadScheme(t *testing.T) {
	setupTest(t)

	r1, _ := http.NewRequest("GET", "ftp://test.com", nil)

	proxies := &pkgconfigmodel.Proxy{
		HTTP: "http://test.com",
	}
	c := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	proxyFunc := GetProxyTransportFunc(proxies, c)

	proxyURL, err := proxyFunc(r1)
	assert.Nil(t, err)
	assert.Nil(t, proxyURL)
}

func TestCreateHTTPTransport(t *testing.T) {
	c := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))

	c.SetWithoutSource("skip_ssl_validation", false)
	transport := CreateHTTPTransport(c)
	assert.False(t, transport.TLSClientConfig.InsecureSkipVerify)
	assert.Equal(t, transport.TLSClientConfig.MinVersion, uint16(tls.VersionTLS12))

	c.SetWithoutSource("skip_ssl_validation", true)
	transport = CreateHTTPTransport(c)
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	assert.Equal(t, transport.TLSClientConfig.MinVersion, uint16(tls.VersionTLS12))

	transport = CreateHTTPTransport(c)
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	assert.Equal(t, transport.TLSClientConfig.MinVersion, uint16(tls.VersionTLS12))
}

func TestNoProxyWarningMap(t *testing.T) {
	setupTest(t)

	r1, _ := http.NewRequest("GET", "http://api.test_http.com/api/v1?arg=21", nil)

	proxies := &pkgconfigmodel.Proxy{
		HTTP:    "https://user:pass@proxy.com:3128",
		HTTPS:   "https://user:pass@proxy_https.com:3128",
		NoProxy: []string{"test_http.com"},
	}
	c := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	proxyFunc := GetProxyTransportFunc(proxies, c)

	proxyURL, err := proxyFunc(r1)
	assert.Nil(t, err)
	assert.Equal(t, "https://user:pass@proxy.com:3128", proxyURL.String())

	assert.True(t, noProxyIgnoredWarningMap["http://api.test_http.com"])
}

func TestMinTLSVersionFromConfig(t *testing.T) {
	setupTest(t)

	tests := []struct {
		minTLSVersion string
		expect        uint16
	}{
		{"tlsv1.0", tls.VersionTLS10},
		{"tlsv1.1", tls.VersionTLS11},
		{"tlsv1.2", tls.VersionTLS12},
		{"tlsv1.3", tls.VersionTLS13},
		// case-insensitive
		{"TlSv1.0", tls.VersionTLS10},
		{"TlSv1.3", tls.VersionTLS13},
		// defaults
		{"", tls.VersionTLS12},
		{"", tls.VersionTLS12},
		// invalid values
		{"tlsv1.9", tls.VersionTLS12},
		{"tlsv1.9", tls.VersionTLS12},
		{"blergh", tls.VersionTLS12},
		{"blergh", tls.VersionTLS12},
	}

	for _, test := range tests {
		t.Run(
			fmt.Sprintf("min_tls_version=%s", test.minTLSVersion),
			func(t *testing.T) {
				cfg := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
				if test.minTLSVersion != "" {
					cfg.SetWithoutSource("min_tls_version", test.minTLSVersion)
				}
				got := minTLSVersionFromConfig(cfg)
				require.Equal(t, test.expect, got)
			})
	}
}

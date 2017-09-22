package util

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestEmptyProxy(t *testing.T) {
	r, err := http.NewRequest("GET", "https://test.com", nil)
	require.Nil(t, err)

	proxies := &config.Proxy{}
	proxyFunc := GetProxyTransportFunc(proxies)

	proxyURL, err := proxyFunc(r)
	assert.Nil(t, err)
	assert.Nil(t, proxyURL)
}

func TestHTTPProxy(t *testing.T) {
	rHTTP, _ := http.NewRequest("GET", "http://test.com/api/v1?arg=21", nil)
	rHTTPS, _ := http.NewRequest("GET", "https://test.com/api/v1?arg=21", nil)

	proxies := &config.Proxy{
		HTTP: "https://user:pass@proxy.com:3128",
	}
	proxyFunc := GetProxyTransportFunc(proxies)

	proxyURL, err := proxyFunc(rHTTP)
	assert.Nil(t, err)
	assert.Equal(t, "https://user:pass@proxy.com:3128", proxyURL.String())

	proxyURL, err = proxyFunc(rHTTPS)
	assert.Nil(t, err)
	assert.Nil(t, proxyURL)
}

func TestNoProxy(t *testing.T) {
	r1, _ := http.NewRequest("GET", "http://test_no_proxy.com/api/v1?arg=21", nil)
	r2, _ := http.NewRequest("GET", "http://test_http.com/api/v1?arg=21", nil)
	r3, _ := http.NewRequest("GET", "https://test_https.com/api/v1?arg=21", nil)

	proxies := &config.Proxy{
		HTTP:    "https://user:pass@proxy.com:3128",
		HTTPS:   "https://user:pass@proxy_https.com:3128",
		NoProxy: []string{"test_no_proxy.com", "test.org"},
	}
	proxyFunc := GetProxyTransportFunc(proxies)

	proxyURL, err := proxyFunc(r1)
	assert.Nil(t, err)
	assert.Nil(t, proxyURL)

	proxyURL, err = proxyFunc(r2)
	assert.Nil(t, err)
	assert.Equal(t, "https://user:pass@proxy.com:3128", proxyURL.String())

	proxyURL, err = proxyFunc(r3)
	assert.Nil(t, err)
	assert.Equal(t, "https://user:pass@proxy_https.com:3128", proxyURL.String())
}

func TestErrorParse(t *testing.T) {
	r1, _ := http.NewRequest("GET", "http://test_no_proxy.com/api/v1?arg=21", nil)

	proxies := &config.Proxy{
		HTTP: "21://test.com",
	}
	proxyFunc := GetProxyTransportFunc(proxies)

	proxyURL, err := proxyFunc(r1)
	assert.NotNil(t, err)
	assert.Nil(t, proxyURL)
}

func TestBadScheme(t *testing.T) {
	r1, _ := http.NewRequest("GET", "ftp://test.com", nil)

	proxies := &config.Proxy{
		HTTP: "http://test.com",
	}
	proxyFunc := GetProxyTransportFunc(proxies)

	proxyURL, err := proxyFunc(r1)
	assert.Nil(t, err)
	assert.Nil(t, proxyURL)
}

package util

import (
	"crypto/tls"
	"net/http"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestEmptyProxy(t *testing.T) {
	r, err := http.NewRequest("GET", "https://test.com", nil)
	require.Nil(t, err)

	proxies := &config.Proxy{}
	proxyFunc := getProxyTransportFunc(proxies)

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
	proxyFunc := getProxyTransportFunc(proxies)

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
	proxyFunc := getProxyTransportFunc(proxies)

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
	proxyFunc := getProxyTransportFunc(proxies)

	proxyURL, err := proxyFunc(r1)
	assert.NotNil(t, err)
	assert.Nil(t, proxyURL)
}

func TestBadScheme(t *testing.T) {
	r1, _ := http.NewRequest("GET", "ftp://test.com", nil)

	proxies := &config.Proxy{
		HTTP: "http://test.com",
	}
	proxyFunc := getProxyTransportFunc(proxies)

	proxyURL, err := proxyFunc(r1)
	assert.Nil(t, err)
	assert.Nil(t, proxyURL)
}

func TestCreateHTTPTransport(t *testing.T) {
	skipSSL := config.Datadog.GetBool("skip_ssl_validation")
	forceTLS := config.Datadog.GetBool("force_tls_12")
	defer config.Datadog.Set("skip_ssl_validation", skipSSL)
	defer config.Datadog.Set("force_tls_12", forceTLS)

	config.Datadog.Set("skip_ssl_validation", false)
	config.Datadog.Set("force_tls_12", false)
	transport := CreateHTTPTransport()
	assert.False(t, transport.TLSClientConfig.InsecureSkipVerify)
	assert.Equal(t, transport.TLSClientConfig.MinVersion, uint16(0))

	config.Datadog.Set("skip_ssl_validation", true)
	transport = CreateHTTPTransport()
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	assert.Equal(t, transport.TLSClientConfig.MinVersion, uint16(0))

	config.Datadog.Set("force_tls_12", true)
	transport = CreateHTTPTransport()
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	assert.Equal(t, transport.TLSClientConfig.MinVersion, uint16(tls.VersionTLS12))
}

func TestCreateHTTPTransportEnvVarProxy(t *testing.T) {
	// Test that the transport uses proxy from environment when not defined in config
	transport := CreateHTTPTransport()

	// To compare functions here, we're relying on behavior that's undefined in the golang spec, but works
	// in the Go1 implementation. See https://stackoverflow.com/a/9644797 for details
	assert.Equal(t, reflect.ValueOf(http.ProxyFromEnvironment).Pointer(), reflect.ValueOf(transport.Proxy).Pointer())
}

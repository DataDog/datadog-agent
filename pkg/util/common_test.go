package util

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"

	"gopkg.in/yaml.v2"
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

func TestJSONConverter(t *testing.T) {

	checks := []string{
		"cassandra",
		"kafka",
		"jmx",
		"jmx_alt",
	}

	cache := map[string]integration.RawMap{}
	for _, c := range checks {
		var cf integration.RawMap

		// Read file contents
		yamlFile, err := ioutil.ReadFile(fmt.Sprintf("../collector/corechecks/embed/jmx/fixtures/%s.yaml", c))
		assert.Nil(t, err)

		// Parse configuration
		err = yaml.Unmarshal(yamlFile, &cf)
		assert.Nil(t, err)

		cache[c] = cf
	}

	//convert
	j := map[string]interface{}{}
	c := map[string]interface{}{}
	for name, config := range cache {
		c[name] = GetJSONSerializableMap(config)
	}
	j["configurations"] = c

	//json encode
	_, err := json.Marshal(GetJSONSerializableMap(j))
	assert.Nil(t, err)
}

func TestCreateHTTPTransport(t *testing.T) {
	mockConfig := config.NewMock()

	skipSSL := config.Datadog.GetBool("skip_ssl_validation")
	forceTLS := config.Datadog.GetBool("force_tls_12")
	defer mockConfig.Set("skip_ssl_validation", skipSSL)
	defer mockConfig.Set("force_tls_12", forceTLS)

	mockConfig.Set("skip_ssl_validation", false)
	mockConfig.Set("force_tls_12", false)
	transport := CreateHTTPTransport()
	assert.False(t, transport.TLSClientConfig.InsecureSkipVerify)
	assert.Equal(t, transport.TLSClientConfig.MinVersion, uint16(0))

	mockConfig.Set("skip_ssl_validation", true)
	transport = CreateHTTPTransport()
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	assert.Equal(t, transport.TLSClientConfig.MinVersion, uint16(0))

	mockConfig.Set("force_tls_12", true)
	transport = CreateHTTPTransport()
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	assert.Equal(t, transport.TLSClientConfig.MinVersion, uint16(tls.VersionTLS12))
}

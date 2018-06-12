package util

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	apiKeyReplacement = "api_key=*************************$1"
)

var apiKeyRegExp = regexp.MustCompile("api_key=*\\w+(\\w{5})")

// SanitizeURL sanitizes credentials from a message containing a URL, and returns
// a string that can be logged safely.
// For now, it obfuscates the API key.
func SanitizeURL(message string) string {
	return apiKeyRegExp.ReplaceAllString(message, apiKeyReplacement)
}

// HTTPHeaders returns a http headers including various basic information (User-Agent, Content-Type...).
func HTTPHeaders() map[string]string {
	av, _ := version.New(version.AgentVersion, version.Commit)
	return map[string]string{
		"User-Agent":   fmt.Sprintf("Datadog Agent/%s", av.GetNumber()),
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "text/html, */*",
	}
}

// GetProxyTransportFunc return a proxy function for a http.Transport that
// would return the right proxy depending on the configuration.
func GetProxyTransportFunc(p *config.Proxy) func(*http.Request) (*url.URL, error) {
	return func(r *http.Request) (*url.URL, error) {
		// check no_proxy list first
		for _, host := range p.NoProxy {
			if r.URL.Host == host {
				log.Debugf("URL match no_proxy list item '%s': not using any proxy", host)
				return nil, nil
			}
		}

		// check proxy by scheme
		confProxy := ""
		if r.URL.Scheme == "http" {
			confProxy = p.HTTP
		} else if r.URL.Scheme == "https" {
			confProxy = p.HTTPS
		} else {
			log.Warnf("Proxy configuration do not support scheme '%s'", r.URL.Scheme)
		}

		if confProxy != "" {
			proxyURL, err := url.Parse(confProxy)
			if err != nil {
				err := fmt.Errorf("Could not parse the proxy URL for scheme %s from configuration: %s", r.URL.Scheme, err)
				log.Error(err.Error())
				return nil, err
			}
			userInfo := ""
			if proxyURL.User != nil {
				if _, isSet := proxyURL.User.Password(); isSet {
					userInfo = "*****:*****@"
				} else {
					userInfo = "*****@"
				}
			}

			log.Debugf("Using proxy %s://%s%s for URL '%s'", proxyURL.Scheme, userInfo, proxyURL.Host, SanitizeURL(r.URL.String()))
			return proxyURL, nil
		}

		// no proxy set for this request
		return nil, nil
	}
}

// CreateHTTPTransport creates an *http.Transport for use in the agent
func CreateHTTPTransport() *http.Transport {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.Datadog.GetBool("skip_ssl_validation"),
	}

	if config.Datadog.GetBool("force_tls_12") {
		tlsConfig.MinVersion = tls.VersionTLS12
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	if proxies := config.GetProxies(); proxies != nil {
		transport.Proxy = GetProxyTransportFunc(proxies)
	}
	return transport
}

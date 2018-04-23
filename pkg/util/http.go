package util

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
	log "github.com/cihub/seelog"
)

// HTTPHeaders returns a http headers including various basic information (User-Agent, Content-Type...).
func HTTPHeaders() map[string]string {
	av, _ := version.New(version.AgentVersion, version.Commit)
	return map[string]string{
		"User-Agent":   fmt.Sprintf("Datadog Agent/%s", av.GetNumber()),
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "text/html, */*",
	}
}

// getProxyTransportFunc return a proxy function for a http.Transport that
// would return the right proxy depending on the configuration.
func getProxyTransportFunc(p *config.Proxy) func(*http.Request) (*url.URL, error) {
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

			log.Debugf("Using proxy %s://%s%s for URL '%s'", proxyURL.Scheme, userInfo, proxyURL.Host, r.URL)
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

	if proxies := config.Datadog.Get("proxy"); proxies != nil {
		if os.Getenv("http_proxy") != "" || os.Getenv("https_proxy") != "" ||
			os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" {
			log.Info("Using agent's proxy configuration instead of env vars 'http_proxy', 'https_proxy' and 'no_proxy'")
		}

		proxies := &config.Proxy{}
		if err := config.Datadog.UnmarshalKey("proxy", proxies); err != nil {
			log.Errorf("Could not load the proxy configuration: %s", err)
		} else {
			transport.Proxy = getProxyTransportFunc(proxies)
		}
	} else {
		log.Info("Using proxy settings from environment")
		transport.Proxy = http.ProxyFromEnvironment
	}

	return transport
}

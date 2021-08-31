// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/net/http/httpproxy"
)

var (
	// NoProxyIgnoredWarningMap map containing URL's who will ignore the proxy in the future
	NoProxyIgnoredWarningMap = make(map[string]bool)

	// NoProxyUsedInFuture map containing URL's that will use a proxy in the future
	NoProxyUsedInFuture = make(map[string]bool)

	// NoProxyChanged map containing URL's whos proxy behavior will change in the future
	NoProxyChanged = make(map[string]bool)

	// NoProxyMapMutex Lock for all no proxy maps
	NoProxyMapMutex = sync.Mutex{}
)

func logSafeURLString(url *url.URL) string {
	if url == nil {
		return ""
	}
	return url.Scheme + "://" + url.Host
}

func warnOnce(warnMap map[string]bool, key string, format string, params ...interface{}) {
	NoProxyMapMutex.Lock()
	defer NoProxyMapMutex.Unlock()
	if _, ok := warnMap[key]; !ok {
		warnMap[key] = true
		log.Warnf(format, params...)
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

	// Most of the following timeouts are a copy of Golang http.DefaultTransport
	// They are mostly used to act as safeguards in case we forget to add a general
	// timeout to our http clients.  Setting DialContext and TLSClientConfig has the
	// desirable side-effect of disabling http/2; if removing those fields then
	// consider the implication of the protocol switch for intakes and other http
	// servers. See ForceAttemptHTTP2 in https://pkg.go.dev/net/http#Transport.
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second,
			// Enables TCP keepalives to detect broken connections
			KeepAlive: 30 * time.Second,
			// Disable RFC 6555 Fast Fallback ("Happy Eyeballs")
			FallbackDelay: -1 * time.Nanosecond,
		}).DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 5,
		// This parameter is set to avoid connections sitting idle in the pool indefinitely
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if proxies := config.GetProxies(); proxies != nil {
		transport.Proxy = GetProxyTransportFunc(proxies)
	}

	return transport
}

// GetProxyTransportFunc return a proxy function for a http.Transport that
// would return the right proxy depending on the configuration.
func GetProxyTransportFunc(p *config.Proxy) func(*http.Request) (*url.URL, error) {

	proxyConfig := &httpproxy.Config{
		HTTPProxy:  p.HTTP,
		HTTPSProxy: p.HTTPS,
		NoProxy:    strings.Join(p.NoProxy, ","),
	}

	if config.Datadog.GetBool("no_proxy_nonexact_match") {
		return func(r *http.Request) (*url.URL, error) {
			return proxyConfig.ProxyFunc()(r.URL)
		}
	}

	return func(r *http.Request) (*url.URL, error) {
		url, err := func(r *http.Request) (*url.URL, error) {
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
				logSafeURL := r.URL.Scheme + "://" + r.URL.Host
				log.Debugf("Using proxy %s://%s%s for URL '%s'", proxyURL.Scheme, userInfo, proxyURL.Host, logSafeURL)
				return proxyURL, nil
			}

			// no proxy set for this request
			return nil, nil
		}(r)

		// Test the new proxy function to see if the behavior will change in the future
		newURL, _ := proxyConfig.ProxyFunc()(r.URL)

		if url == nil && newURL == nil {
			return url, err
		}

		logSafeURL := logSafeURLString(r.URL)

		// Print a warning if the url would ignore the proxy when no_proxy_nonexact_match is true
		if url != nil && newURL == nil {
			warnOnce(NoProxyIgnoredWarningMap, logSafeURL, "Deprecation warning: the HTTP request to %s uses proxy %s but will ignore the proxy when the Agent configuration option no_proxy_nonexact_match defaults to true in a future agent version. Please adapt the Agent’s proxy configuration accordingly", logSafeURL, url.String())
			return url, err
		}

		var newURLString string
		if newURL != nil {
			newURLString = newURL.String()
		}

		// There are no known cases that will trigger the below warnings but because they are logically possible we should still include them.

		// Print a warning if the url does not use the proxy - but will for some reason when no_proxy_nonexact_match is true
		if url == nil && newURL != nil {
			warnOnce(NoProxyUsedInFuture, logSafeURL, "Deprecation warning: the HTTP request to %s does not use a proxy but will use: %s when the Agent configuration option no_proxy_nonexact_match defaults to true in a future agent version.", logSafeURL, logSafeURLString(newURL))
			return url, err
		}

		// Print a warning if the url uses the proxy and still will when no_proxy_nonexact_match is true but for some reason is different
		if url.String() != newURLString {
			warnOnce(NoProxyChanged, logSafeURL, "Deprecation warning: the HTTP request to %s uses proxy %s but will change to %s when the Agent configuration option no_proxy_nonexact_match defaults to true", logSafeURL, url.String(), logSafeURLString(newURL))
			return url, err
		}

		return url, err
	}
}

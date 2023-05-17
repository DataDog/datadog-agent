// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http/httpproxy"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

	keyLogWriterInit sync.Once
	keyLogWriter     io.Writer
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

// minTLSVersionFromConfig determines the minimum TLS version defined by the given
// config, accounting for defaults and deprecated configuration parameters.
//
// The returned result is one of the `tls.VersionTLSxxx` constants.
func minTLSVersionFromConfig(cfg config.Config) uint16 {
	var min uint16
	minTLSVersion := cfg.GetString("min_tls_version")
	switch strings.ToLower(minTLSVersion) {
	case "tlsv1.0":
		min = tls.VersionTLS10
	case "tlsv1.1":
		min = tls.VersionTLS11
	case "tlsv1.2":
		min = tls.VersionTLS12
	case "tlsv1.3":
		min = tls.VersionTLS13
	default:
		min = tls.VersionTLS12
		if minTLSVersion != "" {
			log.Warnf("Invalid `min_tls_version` %#v; using default", minTLSVersion)
		}
	}
	return min
}

// CreateHTTPTransport creates an *http.Transport for use in the agent
func CreateHTTPTransport() *http.Transport {
	// It’s OK to reuse the same file for all the http.Transport objects we create
	// because all the writes to that file are protected by a global mutex.
	// See https://github.com/golang/go/blob/go1.17.3/src/crypto/tls/common.go#L1316-L1318
	keyLogWriterInit.Do(func() {
		sslKeyLogFile := config.Datadog.GetString("sslkeylogfile")
		if sslKeyLogFile != "" {
			var err error
			keyLogWriter, err = os.OpenFile(sslKeyLogFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
			if err != nil {
				log.Warnf("Failed to open %s for writing NSS keys: %v", sslKeyLogFile, err)
			}
		}
	})

	tlsConfig := &tls.Config{
		KeyLogWriter:       keyLogWriter,
		InsecureSkipVerify: config.Datadog.GetBool("skip_ssl_validation"),
	}

	tlsConfig.MinVersion = minTLSVersionFromConfig(config.Datadog)

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

	if proxies := config.Datadog.GetProxies(); proxies != nil {
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
					log.Debugf("URL '%s' matches no_proxy list item '%s': not using any proxy", r.URL, host)
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

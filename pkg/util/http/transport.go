// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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
	// NoProxyWarningMap map containing URL's whos proxy behavior will change in the future.
	NoProxyWarningMap = make(map[string]bool)

	// NoProxyWarningMapMutex Lock for NoProxyWarningMap
	NoProxyWarningMapMutex = sync.Mutex{}
)

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
	// timeout to our http clients.
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

		// Print a warning if the proxy behavior would change if the new no_proxy behavior would be enabled
		newURL, _ := proxyConfig.ProxyFunc()(r.URL)
		if url != newURL && url != nil {
			urlString := r.URL.String()
			NoProxyWarningMapMutex.Lock()
			if _, ok := NoProxyWarningMap[urlString]; !ok {
				NoProxyWarningMap[SanitizeURL(r.URL.String())] = true
				logSafeURL := r.URL.Scheme + "://" + r.URL.Host
				log.Warnf("Deprecation warning: the HTTP request to %s uses proxy %s but will ignore the proxy when the Agent configuration option no_proxy_nonexact_match defaults to true in a future agent version. Please adapt the Agent’s proxy configuration accordingly", logSafeURL, url.String())
			}
			NoProxyWarningMapMutex.Unlock()
		}

		return url, err
	}
}

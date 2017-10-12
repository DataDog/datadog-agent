// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package util

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// HTTPHeaders returns a http headers including various basic information (User-Agent, Content-Type...).
func HTTPHeaders() map[string]string {
	av, _ := version.New(version.AgentVersion)
	return map[string]string{
		"User-Agent":   fmt.Sprintf("Datadog Agent/%s", av.GetNumber()),
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "text/html, */*",
	}
}

// GetJSONSerializableMap returns a JSON serializable map from a raw map
func GetJSONSerializableMap(m interface{}) interface{} {
	switch x := m.(type) {
	// unbelievably I cannot collapse this into the next (identical) case
	case map[interface{}]interface{}:
		j := check.ConfigJSONMap{}
		for k, v := range x {
			j[k.(string)] = GetJSONSerializableMap(v)
		}
		return j
	case check.ConfigRawMap:
		j := check.ConfigJSONMap{}
		for k, v := range x {
			j[k.(string)] = GetJSONSerializableMap(v)
		}
		return j
	case check.ConfigJSONMap:
		j := check.ConfigJSONMap{}
		for k, v := range x {
			j[k] = GetJSONSerializableMap(v)
		}
		return j
	case []interface{}:
		j := make([]interface{}, len(x))

		for i, v := range x {
			j[i] = GetJSONSerializableMap(v)
		}
		return j
	}
	return m

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

			log.Debugf("Using proxy %s://%s%s for URL '%s'", proxyURL.Scheme, userInfo, proxyURL.Host, r.URL)
			return proxyURL, nil
		}

		// no proxy set for this request
		return nil, nil
	}
}

// CreateHTTPTransport creates an *http.Transport for use in the agent
func CreateHTTPTransport() *http.Transport {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.Datadog.GetBool("skip_ssl_validation"),
		},
	}

	if proxies := config.Datadog.Get("proxy"); proxies != nil {
		proxies := &config.Proxy{}
		if err := config.Datadog.UnmarshalKey("proxy", proxies); err != nil {
			log.Errorf("Could not load the proxy configuration: %s", err)
		} else {
			transport.Proxy = GetProxyTransportFunc(proxies)
		}
	}

	if os.Getenv("http_proxy") != "" || os.Getenv("https_proxy") != "" ||
		os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" {
		log.Warn("Env variables 'http_proxy' and 'https_proxy' are not enforced by the agent, please use the configuration file.")
	}

	return transport
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package util

import (
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// CopyFile atomically copies file path `src`` to file path `dst`.
func CopyFile(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	perm := fi.Mode()

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp, err := ioutil.TempFile(filepath.Dir(dst), "")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	_, err = io.Copy(tmp, in)
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}

	err = tmp.Close()
	if err != nil {
		os.Remove(tmpName)
		return err
	}

	err = os.Chmod(tmpName, perm)
	if err != nil {
		os.Remove(tmpName)
		return err
	}

	err = os.Rename(tmpName, dst)
	if err != nil {
		os.Remove(tmpName)
		return err
	}

	return nil
}

// CopyFileAll calls CopyFile, but will create necessary directories for  `dst`.
func CopyFileAll(src, dst string) error {
	err := EnsureParentDirsExist(dst)
	if err != nil {
		return err
	}

	return CopyFile(src, dst)
}

// EnsureParentDirsExist makes a path immediately available for
// writing by creating the necessary parent directories.
func EnsureParentDirsExist(p string) error {
	err := os.MkdirAll(filepath.Dir(p), os.ModePerm)
	if err != nil {
		return err
	}

	return nil
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

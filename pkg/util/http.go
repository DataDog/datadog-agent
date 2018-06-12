package util

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

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
		if matches, matchingNoProxy := useProxy(canonicalAddr(r.URL), p.NoProxy); !matches {
			log.Debugf("'%s' matches no_proxy list item '%s': not using any proxy", SanitizeURL(r.URL.String()), matchingNoProxy)
			return nil, nil
		}

		// check proxy by scheme
		confProxy := ""
		if r.URL.Scheme == "http" {
			confProxy = p.HTTP
		} else if r.URL.Scheme == "https" {
			confProxy = p.HTTPS
		} else {
			log.Warnf("Proxy configuration does not support scheme '%s'", r.URL.Scheme)
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

// useProxy reports whether requests to addr should use a proxy,
// according to the noProxy entries, and if a proxy should be used it
// returns the matching noProxy entry.
// addr is always a canonicalAddr with a host and port.
// Copied from `net/http/transport.go`, modified to use the passed noProxy values
// instead of pulling directly from the no_proxy env var, and to return the
// matched noProxy entry.
func useProxy(addr string, noProxy []string) (matches bool, matchingNoProxy string) {
	if len(addr) == 0 {
		return true, ""
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false, "error splitting host port"
	}
	if host == "localhost" {
		return false, "localhost"
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return false, "loopback"
		}
	}

	addr = strings.ToLower(strings.TrimSpace(addr))
	if hasPort(addr) {
		addr = addr[:strings.LastIndex(addr, ":")]
	}

	for _, p := range noProxy {
		p = strings.ToLower(strings.TrimSpace(p))
		matchingNoProxy := p
		if len(p) == 0 {
			continue
		}
		if p == "*" {
			return false, matchingNoProxy
		}
		if hasPort(p) {
			p = p[:strings.LastIndex(p, ":")]
		}
		if addr == p {
			return false, matchingNoProxy
		}
		if len(p) == 0 {
			// There is no host part, likely the entry is malformed; ignore.
			continue
		}
		if p[0] == '.' && (strings.HasSuffix(addr, p) || addr == p[1:]) {
			// no_proxy ".foo.com" matches "bar.foo.com" or "foo.com"
			return false, matchingNoProxy
		}
		if p[0] != '.' && strings.HasSuffix(addr, p) && addr[len(addr)-len(p)-1] == '.' {
			// no_proxy "foo.com" matches "bar.foo.com"
			return false, matchingNoProxy
		}
	}
	return true, ""
}

var portMap = map[string]string{
	"http":   "80",
	"https":  "443",
	"socks5": "1080",
}

// canonicalAddr returns url.Host but always with a ":port" suffix.
// Copied from `net/http/transport.go`, modified to remove the idna conversions.
func canonicalAddr(url *url.URL) string {
	addr := url.Hostname()
	port := url.Port()
	if port == "" {
		port = portMap[url.Scheme]
	}
	return net.JoinHostPort(addr, port)
}

// Given a string of the form "host", "host:port", or "[ipv6::address]:port",
// return true if the string includes a port.
// Copied from `net/http/transport.go`, unaltered.
func hasPort(s string) bool { return strings.LastIndex(s, ":") > strings.LastIndex(s, "]") }

package util

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"

	log "github.com/cihub/seelog"

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

// CreateHTTPTransport creates an *http.Transport for use in the agent
func CreateHTTPTransport() *http.Transport {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.Datadog.GetBool("skip_ssl_validation"),
		},
	}

	if confProxy := config.Datadog.GetString("proxy"); confProxy != "" {
		if proxyURL, err := url.Parse(confProxy); err != nil {
			log.Error("Could not parse the URL in 'proxy' from configuration")
		} else {
			userInfo := ""
			if proxyURL.User != nil {
				if _, isSet := proxyURL.User.Password(); isSet {
					userInfo = "*****:*****@"
				} else {
					userInfo = "*****@"
				}
			}

			log.Infof("Using proxy from configuration over ENV: %s://%s%s", proxyURL.Scheme, userInfo, proxyURL.Host)
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return transport
}

package agentprovider

import (
	"net/url"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/net/publicsuffix"
)

type endpoint struct {
	site    string
	apiKeys []string
}

type configManager struct {
	endpointsTotalLength int
	endpoints []endpoint
	config    config.Component
}

func extractSite(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		log.Debugf("Failed to parse URL %s: %v", s, err)
		return ""
	}

	hostname := strings.Trim(u.Hostname(), ".")
	if hostname == "" {
		return ""
	}
	apexDomain, err := publicsuffix.EffectiveTLDPlusOne(hostname)
	if err != nil {
		log.Debugf("Failed to extract apex domain from %s: %v", hostname, err)
		return hostname
	}

	return apexDomain
}

func newConfigManager(config config.Component) configManager {
	if config == nil {
		return configManager{}
	}

	endpointsTotalLength := 0
	profilingDDURL := config.GetString("apm_config.profiling_dd_url")
	ddSite := config.GetString("site")
	apiKey := config.GetString("api_key")

	var usedSite string
	if profilingDDURL != "" {
		usedSite = extractSite(profilingDDURL)
	} else if ddSite != "" {
		usedSite = ddSite
	}

	profilingAdditionalEndpoints := config.GetStringMapStringSlice("apm_config.profiling_additional_endpoints")
	var endpoints []endpoint
	for endpointURL, keys := range profilingAdditionalEndpoints {
		site := extractSite(endpointURL)
		if site == "" {
			log.Warnf("Could not extract site from URL %s, skipping endpoint", endpointURL)
			continue
		}
		endpoints = append(endpoints, endpoint{
			site:    site,
			apiKeys: keys,
		})
		endpointsTotalLength += len(keys)
	}
	log.Infof("Main site inferred from core configuration is %s", usedSite)

	// Add main endpoint if we have a valid site and API key
	if usedSite != "" && apiKey != "" {
		endpoints = append(endpoints, endpoint{site: usedSite, apiKeys: []string{apiKey}})
		endpointsTotalLength++
	}

	return configManager{config: config, endpoints: endpoints, endpointsTotalLength: endpointsTotalLength}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// InfraURLPrefix is the default infra URL prefix for datadog
	InfraURLPrefix = "https://app."

	// MRFLogsPrefix is the logs-specific MRF site prefix. This is used for both pure logs as well as EvP-based payloads (Database
	// Monitoring, Netflow, etc)
	MRFLogsPrefix = "logs.mrf."

	// MRFInfraPrefix is the infrastructure-specific MRF site prefix. This is used for metadata, metrics, etc.
	MRFInfraPrefix = "mrf."
)

func getResolvedDDUrl(c pkgconfigmodel.Reader, urlKey string) string {
	resolvedDDURL := c.GetString(urlKey)
	if c.IsSet("site") {
		log.Infof("'site' and '%s' are both set in config: setting main endpoint to '%s': \"%s\"", urlKey, urlKey, c.GetString(urlKey))
	}
	return resolvedDDURL
}

// mergeAdditionalEndpoints merges additional endpoints into keysPerDomain
func mergeAdditionalEndpoints(keysPerDomain, additionalEndpoints map[string][]string) (map[string][]string, error) {
	for domain, apiKeys := range additionalEndpoints {
		// Validating domain
		_, err := url.Parse(domain)
		if err != nil {
			return nil, fmt.Errorf("could not parse url from 'additional_endpoints' %s: %s", domain, err)
		}

		if _, ok := keysPerDomain[domain]; ok {
			keysPerDomain[domain] = append(keysPerDomain[domain], apiKeys...)
		} else {
			keysPerDomain[domain] = apiKeys
		}
	}

	// dedupe api keys and remove domains with no api keys (or empty ones)
	for domain, apiKeys := range keysPerDomain {
		dedupedAPIKeys := make([]string, 0, len(apiKeys))
		seen := make(map[string]bool)
		for _, apiKey := range apiKeys {
			trimmedAPIKey := strings.TrimSpace(apiKey)
			if _, ok := seen[trimmedAPIKey]; !ok && trimmedAPIKey != "" {
				seen[trimmedAPIKey] = true
				dedupedAPIKeys = append(dedupedAPIKeys, trimmedAPIKey)
			}
		}

		if len(dedupedAPIKeys) > 0 {
			keysPerDomain[domain] = dedupedAPIKeys
		} else {
			log.Infof("No API key provided for domain \"%s\", removing domain from endpoints", domain)
			delete(keysPerDomain, domain)
		}
	}

	return keysPerDomain, nil
}

// GetMainEndpointBackwardCompatible implements the logic to extract the DD URL from a config, based on `site`,ddURLKey and a backward compatible key
func GetMainEndpointBackwardCompatible(c pkgconfigmodel.Reader, prefix string, ddURLKey string, backwardKey string) string {
	if c.IsSet(ddURLKey) && c.GetString(ddURLKey) != "" {
		// value under ddURLKey takes precedence over backwardKey and 'site'
		return getResolvedDDUrl(c, ddURLKey)
	} else if c.IsSet(backwardKey) && c.GetString(backwardKey) != "" {
		// value under backwardKey takes precedence over 'site'
		return getResolvedDDUrl(c, backwardKey)
	} else if c.GetString("site") != "" {
		return prefix + strings.TrimSpace(c.GetString("site"))
	}
	return prefix + pkgconfigsetup.DefaultSite
}

// GetMultipleEndpoints returns the api keys per domain specified in the main agent config
func GetMultipleEndpoints(c pkgconfigmodel.Reader) (map[string][]string, error) {
	ddURL := GetInfraEndpoint(c)
	// Validating domain
	if _, err := url.Parse(ddURL); err != nil {
		return nil, fmt.Errorf("could not parse main endpoint: %s", err)
	}

	keysPerDomain := map[string][]string{
		ddURL: {
			c.GetString("api_key"),
		},
	}

	additionalEndpoints := c.GetStringMapStringSlice("additional_endpoints")

	// populate with MRF endpoints too
	if c.GetBool("multi_region_failover.enabled") {
		haURL, err := GetMRFInfraEndpoint(c)
		if err != nil {
			return nil, fmt.Errorf("could not parse MRF endpoint: %s", err)
		}
		additionalEndpoints[haURL] = []string{c.GetString("multi_region_failover.api_key")}
	}
	return mergeAdditionalEndpoints(keysPerDomain, additionalEndpoints)
}

// BuildURLWithPrefix will return an HTTP(s) URL for a site given a certain prefix
func BuildURLWithPrefix(prefix, site string) string {
	return prefix + strings.TrimSpace(site)
}

// GetMainEndpoint returns the main DD URL defined in the config, based on `site` and the prefix, or ddURLKey
func GetMainEndpoint(c pkgconfigmodel.Reader, prefix string, ddURLKey string) string {
	// value under ddURLKey takes precedence over 'site'
	if c.IsSet(ddURLKey) && c.GetString(ddURLKey) != "" {
		return getResolvedDDUrl(c, ddURLKey)
	} else if c.GetString("site") != "" {
		return BuildURLWithPrefix(prefix, c.GetString("site"))
	}
	return BuildURLWithPrefix(prefix, pkgconfigsetup.DefaultSite)
}

// GetMRFEndpoint returns the generic MRF endpoint to use.
//
// This is based on the `multi_region_failover.site` setting. If `ddMRFURLKey` is not empty, we attempt to use it as a
// lookup key in the configuration. If a valid is set at the given key, it is used as an override URL that takes
// precedence over `multi_region_failover.site`.
func GetMRFEndpoint(c pkgconfigmodel.Reader, prefix, ddMRFURLKey string) (string, error) {
	if c.IsSet(ddMRFURLKey) && c.GetString(ddMRFURLKey) != "" {
		return getResolvedMRFDDURL(c, ddMRFURLKey), nil
	} else if c.GetString("multi_region_failover.site") != "" {
		return BuildURLWithPrefix(prefix, c.GetString("multi_region_failover.site")), nil
	}
	return "", fmt.Errorf("`multi_region_failover.site` or `%s` must be set when Multi-Region Failover is enabled", ddMRFURLKey)
}

// GetMRFLogsEndpoint returns the logs-specific MRF endpoint to use.
//
// This is based on the `multi_region_failover.site` setting. If `ddMRFURLKey` is not empty, we attempt to use it as a
// lookup key in the configuration. If a valid is set at the given key, it is used as an override URL that takes
// precedence over `multi_region_failover.site`.
func GetMRFLogsEndpoint(c pkgconfigmodel.Reader, prefix string) (string, error) {
	// For pure logs, we already use a prefix that looks like `agent-http-intake.logs.`, but for other EvP intake
	// tracks, they just have a generic prefix that looks like the product name (e.g., `dbm-metrics-intake.`), so we
	// only want to append the `.logs.` suffix if it's not already present.
	logsSpecificPrefix := prefix + MRFInfraPrefix
	if !strings.HasSuffix(prefix, ".logs.") {
		logsSpecificPrefix = prefix + MRFLogsPrefix
	}

	return GetMRFEndpoint(c, logsSpecificPrefix, "multi_region_failover.dd_url")
}

func getResolvedMRFDDURL(c pkgconfigmodel.Reader, mrfURLKey string) string {
	resolvedMRFDDURL := c.GetString(mrfURLKey)
	if c.IsSet("multi_region_failover.site") {
		log.Infof("'multi_region_failover.site' and '%s' are both set in config: setting main endpoint to '%s': \"%s\"", mrfURLKey, mrfURLKey, resolvedMRFDDURL)
	}
	return resolvedMRFDDURL
}

// GetInfraEndpoint returns the main DD Infra URL defined in config, based on the value of `site` and `dd_url`
func GetInfraEndpoint(c pkgconfigmodel.Reader) string {
	return GetMainEndpoint(c, InfraURLPrefix, "dd_url")
}

// GetMRFInfraEndpoint returns the infrastructure-specific MRF endpoint to use.
//
// This is based on the `multi_region_failover.site` setting. If `ddMRFURLKey` is not empty, we attempt to use it as a
// lookup key in the configuration. If a valid is set at the given key, it is used as an override URL that takes
// precedence over `multi_region_failover.site`.
func GetMRFInfraEndpoint(c pkgconfigmodel.Reader) (string, error) {
	fullInfraURLPrefix := InfraURLPrefix + MRFInfraPrefix
	return GetMRFEndpoint(c, fullInfraURLPrefix, "multi_region_failover.dd_url")
}

// ddURLRegexp determines if an URL belongs to Datadog or not. If the URL belongs to Datadog it's prefixed with the Agent
// version (see AddAgentVersionToDomain).
var ddURLRegexp = regexp.MustCompile(`^app(\.mrf)?(\.[a-z]{2}\d)?\.(datad(oghq|0g)\.(com|eu)|ddog-gov\.com)$`)

// getDomainPrefix provides the right prefix for agent X.Y.Z
func getDomainPrefix(app string) string {
	v, _ := version.Agent()
	return fmt.Sprintf("%d-%d-%d-%s.agent", v.Major, v.Minor, v.Patch, app)
}

// AddAgentVersionToDomain prefixes the domain with the agent version: X-Y-Z.domain
func AddAgentVersionToDomain(DDURL string, app string) (string, error) {
	u, err := url.Parse(DDURL)
	if err != nil {
		return "", err
	}

	// we don't update unknown URLs (ie: proxy or custom DD domain)
	if !ddURLRegexp.MatchString(u.Host) {
		return DDURL, nil
	}

	subdomain := strings.Split(u.Host, ".")[0]
	newSubdomain := getDomainPrefix(app)

	u.Host = strings.Replace(u.Host, subdomain, newSubdomain, 1)
	return u.String(), nil
}

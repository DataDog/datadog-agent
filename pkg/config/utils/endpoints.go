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

	"golang.org/x/net/idna"

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
		log.Debugf("'site' and '%s' are both set in config: setting main endpoint to '%s': \"%s\"", urlKey, urlKey, c.GetString(urlKey))
	}
	return resolvedDDURL
}

// APIKeys contains a list of API keys together with the path within the config that this API key were configured.
type APIKeys struct {
	// The path of the config used to get the API key. This path is used to listen for configuration updates from
	// the config.
	ConfigSettingPath string

	// the apiKey to use for this endpoint
	Keys []string
}

// NewAPIKeys creates an endpoint
func NewAPIKeys(path string, keys ...string) APIKeys {
	return APIKeys{
		ConfigSettingPath: path,
		Keys:              keys,
	}
}

func newAPIKeyset(path string, keys ...string) []APIKeys {
	return []APIKeys{NewAPIKeys(path, keys...)}
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

// MakeEndpoints takes a map of domain to apikeys and a config path root and converts this to
// a map of domain to Endpoint structs.
func MakeEndpoints(endpoints map[string][]string, root string) map[string][]APIKeys {
	result := map[string][]APIKeys{}
	for url, keys := range endpoints {
		// Remove any empty API keys.
		// We don't need to hold on to an endpoint with an empty API key to track if a
		// secret has been updated since secrets can never be empty in the first place.
		trimmed := []string{}
		for _, key := range keys {
			trimmedAPIKey := strings.TrimSpace(key)
			if trimmedAPIKey != "" {
				trimmed = append(trimmed, trimmedAPIKey)
			}
		}

		if len(trimmed) > 0 {
			result[url] = []APIKeys{{
				ConfigSettingPath: root,
				Keys:              trimmed,
			}}
		} else {
			log.Infof("No API key provided for domain %q, removing domain from endpoints", url)
		}
	}

	return result
}

// DedupAPIKeys takes a single array of endpoints and returns an array of unique
// api keys that they contain.
// This needs to be a separate process to loading because we need to keep track
// of the endpoints with the API config location to know when they have been
// refreshed.
func DedupAPIKeys(endpoints []APIKeys) []string {
	dedupedAPIKeys := make([]string, 0)
	seen := make(map[string]bool)
	for _, endpoint := range endpoints {
		for _, apiKey := range endpoint.Keys {
			if _, ok := seen[apiKey]; !ok {
				seen[apiKey] = true
				dedupedAPIKeys = append(dedupedAPIKeys, apiKey)
			}
		}
	}

	return dedupedAPIKeys
}

// EndpointDescriptor holds configuration about a single endpoint (aka domain) for infra pipelines.
type EndpointDescriptor struct {
	BaseURL   string
	APIKeySet []APIKeys
	IsMRF     bool
}

func newEndpointDescriptor(baseURL string, apiKeySet []APIKeys) EndpointDescriptor {
	return EndpointDescriptor{
		BaseURL:   baseURL,
		APIKeySet: apiKeySet,
	}
}

// EndpointDescriptorSet is a collection of all endpoints for infra pipelines keyed by base URL.
type EndpointDescriptorSet = map[string]EndpointDescriptor

// EndpointDescriptorSetFromKeysPerDomain converts legacy endpoint configuration into EndpointDescriptorSet.
func EndpointDescriptorSetFromKeysPerDomain(keysPerDomain map[string][]APIKeys) EndpointDescriptorSet {
	eds := EndpointDescriptorSet{}
	for domain, keyset := range keysPerDomain {
		eds[domain] = newEndpointDescriptor(domain, keyset)
	}

	return eds
}

// GetMultipleEndpoints returns the api keys per domain specified in the main agent config
func GetMultipleEndpoints(c pkgconfigmodel.Reader) (EndpointDescriptorSet, error) {
	ddURL := GetInfraEndpoint(c)
	// Validating domain
	if _, err := url.Parse(ddURL); err != nil {
		return nil, fmt.Errorf("could not parse main endpoint: %s", err)
	}

	keysPerDomain := map[string][]APIKeys{
		ddURL: newAPIKeyset("api_key", c.GetString("api_key")),
	}

	additionalEndpoints := MakeEndpoints(c.GetStringMapStringSlice("additional_endpoints"), "additional_endpoints")

	for domain, apiKeys := range additionalEndpoints {
		// Validating domain
		_, err := url.Parse(domain)
		if err != nil {
			return nil, fmt.Errorf("could not parse url from 'additional_endpoints' %s: %s", domain, err)
		}

		if oldAPIKeys, ok := keysPerDomain[domain]; ok {
			keysPerDomain[domain] = append(oldAPIKeys, apiKeys...)
		} else {
			keysPerDomain[domain] = apiKeys
		}
	}

	eds := EndpointDescriptorSetFromKeysPerDomain(keysPerDomain)

	// populate with MRF endpoints too
	if c.GetBool("multi_region_failover.enabled") {
		haURL, err := GetMRFInfraEndpoint(c)
		if err != nil {
			return nil, fmt.Errorf("could not parse MRF endpoint: %s", err)
		}
		ed := newEndpointDescriptor(
			haURL,
			newAPIKeyset("multi_region_failover.api_key", c.GetString("multi_region_failover.api_key")))
		ed.IsMRF = true
		eds[haURL] = ed
	}

	return eds, nil
}

// ddDomainPattern matches known Datadog domains (e.g., datadoghq.com,
// datad0g.eu, ddog-gov.com). This is the shared building block for
// wellKnownSitesRe, ddSitePattern, ddSiteFromHostnameRe, and ddURLRegexp.
const ddDomainPattern = `datad(?:oghq|0g)\.(?:com|eu)|ddog-gov\.com`

var wellKnownSitesRe = regexp.MustCompile(`(?:` + ddDomainPattern + `)$`)

// ddSitePattern matches a Datadog site: an optional datacenter subdomain
// (e.g., us3, ap1) followed by a known Datadog domain.
const ddSitePattern = `([a-z]{2,}\d{1,2}\.)?(` + ddDomainPattern + `)`

// ddSiteFromHostnameRe extracts the Datadog site from the end of a hostname.
// The (?:^|\.) prefix ensures the match starts at a label boundary so that,
// e.g., "notdatadoghq.com" is not mistaken for "datadoghq.com".
var ddSiteFromHostnameRe = regexp.MustCompile(`(?:^|\.)` + ddSitePattern + `\.?$`)

// ExtractSiteFromURL extracts the Datadog site from a URL.
// For example:
//
//	"https://intake.profile.us3.datadoghq.com/v1/input" returns "us3.datadoghq.com"
//	"https://intake.profile.datadoghq.com/v1/input" returns "datadoghq.com"
//	"https://intake.profile.datadoghq.eu/v1/input" returns "datadoghq.eu"
//
// Returns an empty string if the URL cannot be parsed or does not contain a
// recognized Datadog domain.
func ExtractSiteFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	hostname := strings.ToLower(strings.TrimRight(u.Hostname(), "."))
	if hostname == "" {
		return ""
	}

	matches := ddSiteFromHostnameRe.FindStringSubmatch(hostname)
	if matches == nil {
		return ""
	}
	// matches[1] is the DC label with trailing dot (e.g., "us3.") or empty
	// matches[2] is the known domain (e.g., "datadoghq.com")
	return matches[1] + matches[2]
}

// BuildURLWithPrefix will return an HTTP(s) URL for a site given a certain prefix.
// If the site is a datadog well-known one, it is suffixed with a dot to make it a FQDN.
// Using FQDN will prevent useless DNS queries built with the search domains of `/etc/resolv.conf`.
// https://docs.datadoghq.com/getting_started/site/#access-the-datadog-site
func BuildURLWithPrefix(prefix, site string) string {
	site = strings.TrimSpace(site)
	if normalized, err := idna.Lookup.ToASCII(site); err == nil {
		site = normalized
	}
	if pkgconfigsetup.Datadog().GetBool("convert_dd_site_fqdn.enabled") && wellKnownSitesRe.MatchString(site) && !strings.HasSuffix(site, ".") {
		site += "."
	}
	return prefix + site
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
var ddURLRegexp = regexp.MustCompile(`^app(\.mrf)?\.` + ddSitePattern + `\.?$`)

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

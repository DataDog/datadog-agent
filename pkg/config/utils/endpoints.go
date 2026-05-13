// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/idna"

	secretutils "github.com/DataDog/datadog-agent/comp/core/secrets/utils"
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

// getResolvedURL returns the URL stored at urlKey, logging a notice when
// siteKey is also set so users see that the explicit URL is taking precedence.
func getResolvedURL(c pkgconfigmodel.Reader, urlKey, siteKey string) string {
	resolved := c.GetString(urlKey)
	if c.IsSet(siteKey) {
		log.Debugf("'%s' and '%s' are both set in config: setting main endpoint to '%s': %q", siteKey, urlKey, urlKey, resolved)
	}
	return resolved
}

// APIKey contains one API key value and its stable, non-secret identity.
type APIKey struct {
	// Key is the API key material to use for this endpoint.
	Key string

	// Name is the stable, non-secret identity for Key.
	Name string
}

// APIKeys contains a list of API keys together with the path within the config that this API key were configured.
type APIKeys struct {
	// The path of the config used to get the API key. This path is used to listen for configuration updates from
	// the config.
	ConfigSettingPath string

	// Keys contains the API keys to use for this endpoint.
	Keys []APIKey
}

// NewAPIKeys creates an APIKeys for the given config setting path and endpoint.
// Each returned APIKey gets a stable name derived from endpoint and its
// position in keys (see apiKeyNameForIndex), with ENC[...]
// handles preserved when present. Empty/whitespace-only keys are dropped, and
// indexes in the remaining names reflect the original list positions so adding
// or removing empty entries does not renumber the survivors.
func NewAPIKeys(path, endpoint string, keys ...string) APIKeys {
	return APIKeys{
		ConfigSettingPath: path,
		Keys:              makeNamedAPIKeys(endpoint, path, keys, nil),
	}
}

// makeNamedAPIKeys builds the APIKey list for one endpoint at a single config
// setting path. rawKeys is the unresolved (pre-secret-resolution) form of
// keys; pass nil when not available. Secret-backed entries (ENC[handle])
// receive enc_<endpoint>_<path>_<handle> names so rotation preserves identity;
// plain entries receive idx_<endpoint>_<path>_<position>. Duplicate key values
// within keys are collapsed to a single entry, keeping the first occurrence's
// name.
func makeNamedAPIKeys(endpoint, path string, keys, rawKeys []string) []APIKey {
	result := make([]APIKey, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for index, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		result = append(result, APIKey{
			Key:  trimmed,
			Name: additionalEndpointAPIKeyName(endpoint, path, index, key, rawKeys),
		})
	}
	return result
}

const (
	plainAPIKeyNamePrefix  = "idx"
	secretAPIKeyNamePrefix = "enc"
	apiKeyNameSeparator    = "_"
)

// apiKeyNameForIndex returns the stable name for a plain API key at original
// list position index, scoped by endpoint and config setting path. The
// endpoint is canonicalized so equivalent spellings produce the same
// identifier; the agent-version prefix is intentionally not applied so the
// name stays stable across upgrades. Including path disambiguates names when
// two APIKeys sets (e.g. api_key and additional_endpoints) target the same
// endpoint.
func apiKeyNameForIndex(endpoint, path string, index int) string {
	return strings.Join([]string{
		plainAPIKeyNamePrefix,
		canonicalEndpoint(endpoint),
		path,
		strconv.Itoa(index),
	}, apiKeyNameSeparator)
}

// apiKeyNameForSecret returns the stable name for an API key backed by the
// given ENC[handle], scoped by endpoint and config setting path.
func apiKeyNameForSecret(endpoint, path, handle string) string {
	return strings.Join([]string{
		secretAPIKeyNamePrefix,
		canonicalEndpoint(endpoint),
		path,
		handle,
	}, apiKeyNameSeparator)
}

// canonicalEndpoint normalizes a user-configured endpoint so that equivalent
// spellings (with/without scheme, trailing slash, scheme/host case) share the
// same identifier. The agent-version prefix added by AddAgentVersionToDomain
// is a request-time concern and is intentionally not applied here.
func canonicalEndpoint(endpoint string) string {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return ""
	}
	raw := trimmed
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return trimmed
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		scheme = "https"
	}
	host := strings.ToLower(u.Host)
	path := strings.TrimRight(u.Path, "/")
	return scheme + "://" + host + path
}

// GetMainEndpointBackwardCompatible implements the logic to extract the DD URL from a config, based on `site`,ddURLKey and a backward compatible key
func GetMainEndpointBackwardCompatible(c pkgconfigmodel.Reader, prefix string, ddURLKey string, backwardKey string) string {
	if c.IsSet(ddURLKey) && c.GetString(ddURLKey) != "" {
		// value under ddURLKey takes precedence over backwardKey and 'site'
		return getResolvedURL(c, ddURLKey, "site")
	} else if c.IsSet(backwardKey) && c.GetString(backwardKey) != "" {
		// value under backwardKey takes precedence over 'site'
		return getResolvedURL(c, backwardKey, "site")
	} else if c.GetString("site") != "" {
		return prefix + strings.TrimSpace(c.GetString("site"))
	}
	return prefix + pkgconfigsetup.DefaultSite
}

// MakeNamedEndpoints takes a map of domain to apikeys and a config path root
// and produces one APIKeys per non-empty domain. rawEndpoints can carry the
// original unresolved config values so secret-backed names keep their ENC[...]
// handle across rotation; pass nil when not available. Endpoints with only
// empty/whitespace keys are dropped with a log line.
func MakeNamedEndpoints(endpoints map[string][]string, rawEndpoints map[string][]string, root string) map[string][]APIKeys {
	result := map[string][]APIKeys{}
	for url, keys := range endpoints {
		named := makeNamedAPIKeys(url, root, keys, rawEndpoints[url])
		if len(named) == 0 {
			log.Infof("No API key provided for domain %q, removing domain from endpoints", url)
			continue
		}
		result[url] = []APIKeys{{ConfigSettingPath: root, Keys: named}}
	}
	return result
}

// additionalEndpointAPIKeyName returns the stable name for the API key at
// list position index on endpoint+path. It prefers the raw config value (which
// still contains ENC[handle] before secret resolution), and falls back to the
// resolved value when no raw config is available. This keeps secret-backed
// names stable across rotation.
func additionalEndpointAPIKeyName(endpoint, path string, index int, resolvedKey string, rawKeys []string) string {
	source := resolvedKey
	if index < len(rawKeys) {
		source = rawKeys[index]
	}
	if ok, handle := secretutils.IsEnc(source); ok {
		return apiKeyNameForSecret(endpoint, path, handle)
	}
	return apiKeyNameForIndex(endpoint, path, index)
}

// AdditionalEndpointsWithNames returns configstream's serialized shape for
// additional_endpoints: endpoint -> [{"name": stableName, "key": resolvedKey}].
func AdditionalEndpointsWithNames(c pkgconfigmodel.Reader, setting string, value interface{}) map[string][]map[string]string {
	resolved := getStringMapStringSlice(value)
	if resolved == nil {
		resolved = c.GetStringMapStringSlice(setting)
	}
	named := MakeNamedEndpoints(resolved, RawStringMapStringSlice(c, setting), setting)

	result := make(map[string][]map[string]string, len(named))
	for endpoint, apiKeys := range named {
		for _, set := range apiKeys {
			for _, k := range set.Keys {
				result[endpoint] = append(result[endpoint], map[string]string{
					"name": k.Name,
					"key":  k.Key,
				})
			}
		}
	}
	return result
}

// RawStringMapStringSlice returns the user-configured value of setting with
// the secret layer excluded, so callers can see ENC[handle] entries as the
// user wrote them instead of the resolved key material.
func RawStringMapStringSlice(c pkgconfigmodel.Reader, setting string) map[string][]string {
	return getStringMapStringSlice(c.AllSettingsWithoutSecrets()[setting])
}

// getStringMapStringSlice normalizes a setting value to map[string][]string.
// Both viper and nodetreemodel normalize YAML's map[interface{}]interface{}
// to map[string]interface{} at parse time, and env-var JSON parsing produces
// the same shape natively, so a single case covers both sources.
func getStringMapStringSlice(value interface{}) map[string][]string {
	raw, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	result := make(map[string][]string, len(raw))
	for endpoint, rawKeys := range raw {
		keys, ok := interfaceToStringSlice(rawKeys)
		if !ok {
			continue
		}
		result[endpoint] = keys
	}
	return result
}

func interfaceToStringSlice(value interface{}) ([]string, bool) {
	raw, ok := value.([]interface{})
	if !ok {
		return nil, false
	}
	keys := make([]string, 0, len(raw))
	for _, rawKey := range raw {
		key, ok := rawKey.(string)
		if !ok {
			return nil, false
		}
		keys = append(keys, key)
	}
	return keys, true
}

// DedupAPIKeys returns the unique APIKey entries across all endpoints,
// preserving the first occurrence of each key value. Names ride along on the
// APIKey struct so callers don't manage parallel slices.
func DedupAPIKeys(endpoints []APIKeys) []APIKey {
	result := make([]APIKey, 0)
	seen := make(map[string]bool)
	for _, endpoint := range endpoints {
		for _, apiKey := range endpoint.Keys {
			if !seen[apiKey.Key] {
				seen[apiKey.Key] = true
				result = append(result, apiKey)
			}
		}
	}
	return result
}

// EndpointDescriptor holds configuration about a single endpoint (aka domain) for infra pipelines.
type EndpointDescriptor struct {
	BaseURL   string
	APIKeySet []APIKeys
	IsMRF     bool
}

// EndpointDescriptorSet is a collection of all endpoints for infra pipelines keyed by base URL.
type EndpointDescriptorSet = map[string]EndpointDescriptor

// EndpointDescriptorSetFromKeysPerDomain converts legacy endpoint configuration into EndpointDescriptorSet.
func EndpointDescriptorSetFromKeysPerDomain(keysPerDomain map[string][]APIKeys) EndpointDescriptorSet {
	eds := EndpointDescriptorSet{}
	for domain, keyset := range keysPerDomain {
		eds[domain] = EndpointDescriptor{BaseURL: domain, APIKeySet: keyset}
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
		ddURL: {NewAPIKeys("api_key", ddURL, c.GetString("api_key"))},
	}

	additionalEndpoints := MakeNamedEndpoints(
		c.GetStringMapStringSlice("additional_endpoints"),
		RawStringMapStringSlice(c, "additional_endpoints"),
		"additional_endpoints",
	)

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
		eds[haURL] = EndpointDescriptor{
			BaseURL:   haURL,
			APIKeySet: []APIKeys{NewAPIKeys("multi_region_failover.api_key", haURL, c.GetString("multi_region_failover.api_key"))},
			IsMRF:     true,
		}
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
		return getResolvedURL(c, ddURLKey, "site")
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
	if c.IsConfigured(ddMRFURLKey) && c.GetString(ddMRFURLKey) != "" {
		return getResolvedURL(c, ddMRFURLKey, "multi_region_failover.site"), nil
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

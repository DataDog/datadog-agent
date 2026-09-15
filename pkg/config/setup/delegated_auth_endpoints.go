// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	delaOrgUUIDRe       = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	delaNameRe          = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	delaRegionRe        = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	delaFallbackValueRe = regexp.MustCompile(`^[A-Za-z0-9_=-]+$`)
)

// delaDirective is a parsed DELA(...) directive found in an `additional_endpoints` value.
type delaDirective struct {
	orgUUID  string
	provider string
	// Parameter names are case-insensitive.
	params map[string]string
}

// parseDelaDirective parses DELA(<org_uuid>, <provider>[, <name>=<value>]...).
// Whitespace is allowed around separators. Supported parameters are region and fallback.
func parseDelaDirective(value string) (delaDirective, bool) {
	if strings.ContainsAny(value, "\r\n") {
		return delaDirective{}, false
	}
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, pkgconfigmodel.DelaDirectivePrefix) || !strings.HasSuffix(value, ")") {
		return delaDirective{}, false
	}

	body := value[len(pkgconfigmodel.DelaDirectivePrefix) : len(value)-1]
	parts := strings.Split(body, ",")
	if len(parts) < 2 {
		return delaDirective{}, false
	}

	orgUUID := strings.TrimSpace(parts[0])
	provider := strings.TrimSpace(parts[1])
	if !delaOrgUUIDRe.MatchString(orgUUID) || !delaNameRe.MatchString(provider) {
		return delaDirective{}, false
	}

	params := map[string]string{}
	for _, pair := range parts[2:] {
		key, val, found := strings.Cut(strings.TrimSpace(pair), "=")
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		if !found || !delaNameRe.MatchString(key) {
			return delaDirective{}, false
		}
		switch key {
		case "region":
			if !delaRegionRe.MatchString(val) {
				return delaDirective{}, false
			}
		case "fallback":
			if !delaFallbackValueRe.MatchString(val) {
				return delaDirective{}, false
			}
		default:
			return delaDirective{}, false
		}
		if _, duplicate := params[key]; duplicate {
			return delaDirective{}, false
		}
		params[key] = val
	}

	return delaDirective{orgUUID: orgUUID, provider: strings.ToLower(provider), params: params}, true
}

// providerConfigForDirective builds a ProviderConfig for a DELA(...) directive, falling back to
// the process-wide default when the directive omits provider-specific overrides.
//
// Returns nil when neither the directive nor the default supplies provider-specific config (e.g.
// an AWS directive with no region and no process-wide delegated_auth.aws.region). A nil config
// tells the component to auto-detect, which is the right behavior for a directive that says only
// "DELA(org, aws)" — the region should come from the runtime environment, not default to empty.
func providerConfigForDirective(directive delaDirective, defaultProviderConfig common.ProviderConfig) (common.ProviderConfig, error) {
	switch directive.provider {
	case cloudauthconfig.ProviderAWS:
		region := directive.params["region"]
		if region == "" {
			if awsConfig, ok := defaultProviderConfig.(*cloudauthconfig.AWSProviderConfig); ok {
				region = awsConfig.Region
			}
		}
		if region == "" {
			// No explicit region: let the component auto-detect from the environment.
			return nil, nil
		}
		return &cloudauthconfig.AWSProviderConfig{Region: region}, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q in DELA(...) directive", directive.provider)
	}
}

// mapShapeDelegatedAuthEndpointKeys lists the map-shaped settings enabled for delegated-auth
// write-back. Keep this list small: each consumer must ignore DELA(...) until the key is written.
//
// Settings reached through pkg/config/utils.MakeEndpoints satisfy (a) already, because
// PartitionRealAndPendingKeys strips directives while keeping the domain alive.
var mapShapeDelegatedAuthEndpointKeys = []string{
	// Read by pkg/config/utils.GetMultipleEndpoints and served by comp/forwarder/defaultforwarder,
	// which covers metrics, events, service checks and everything else on the main forwarder.
	"additional_endpoints",
}

// listShapeDelegatedAuthEndpointKeys lists the list-shaped settings enabled for write-back.
var listShapeDelegatedAuthEndpointKeys = []string{
	// Supported for HTTP logs endpoints. TCP endpoints skip pending directives.
	"logs_config.additional_endpoints",
}

// configureListShapeAdditionalEndpointsDelegatedAuth scans the list-shape additional_endpoints
// settings for DELA(...) directives in each entry's api_key and registers an instance per match.
func configureListShapeAdditionalEndpointsDelegatedAuth(ctx context.Context, config pkgconfigmodel.Config, delegatedAuthComp delegatedauth.Component, defaultProviderConfig common.ProviderConfig) {
	for _, configKey := range listShapeDelegatedAuthEndpointKeys {
		entries, _ := common.NormalizeListShapeEntries(config.Get(configKey))

		for index, rawEntry := range entries {
			// NormalizeListShapeEntries keeps non-map elements as-is - a bare string, or nil from a
			// blank YAML list item - so they round-trip unchanged. None can hold a directive.
			entry, ok := rawEntry.(map[string]any)
			if !ok {
				continue
			}

			_, directive, ok := common.CaseInsensitiveStringFieldWithKey(entry, "api_key")
			if !ok || !strings.HasPrefix(strings.TrimSpace(directive), pkgconfigmodel.DelaDirectivePrefix) {
				continue
			}
			if !config.GetBool("logs_config.use_http") && !config.GetBool("logs_config.force_use_http") {
				log.Warnf("Additional endpoint entry %d at %q uses delegated auth, which requires HTTP logs transport; set logs_config.force_use_http to enable it", index, configKey)
				continue
			}

			// The entry's host is both the destination the consumer looks the provider up by and
			// the site the auth proof is exchanged against. Without it the exchange would silently
			// fall back to the agent's primary site and fail for a different org.
			host, hasHost := common.CaseInsensitiveStringField(entry, "host")
			if !hasHost || strings.TrimSpace(host) == "" {
				log.Warnf("Additional endpoint entry %d at %q has a delegated auth directive but no host; it cannot be matched to a credential and will not send", index, configKey)
				continue
			}
			identity, ok := common.ListEntryIdentity(entry)
			if !ok {
				log.Warnf("Additional endpoint entry %d at %q could not be fingerprinted; delegated auth is disabled for it", index, configKey)
				continue
			}

			addDelegatedAuthEndpointInstance(ctx, config, delegatedAuthComp, defaultProviderConfig, directive,
				fmt.Sprintf("additional endpoint entry %d (%q) at %q", index, host, configKey),
				delegatedauth.InstanceParams{
					APIKeyConfigKey:                  fmt.Sprintf("%s[%d]", configKey, index),
					AdditionalEndpointsListConfigKey: configKey,
					ListEntryIndex:                   index,
					AdditionalEndpointIdentity:       identity,
					TargetSite:                       host,
				})
		}
	}
}

// configureAdditionalEndpointsDelegatedAuth scans the supported additional_endpoints settings for
// DELA(...) directives and registers a delegated-auth instance for each one.
//
// The resolved key is written into the exact slot that held the directive. Existing config update
// callbacks then deliver it through the normal static-key path.
func configureAdditionalEndpointsDelegatedAuth(ctx context.Context, config pkgconfigmodel.Config, delegatedAuthComp delegatedauth.Component, defaultProviderConfig common.ProviderConfig) {
	for _, configKey := range mapShapeDelegatedAuthEndpointKeys {
		for domain, keys := range config.GetStringMapStringSlice(configKey) {
			if strings.TrimSpace(domain) == "" {
				log.Warnf("Additional endpoint at %q has no domain; delegated auth is disabled for it", configKey)
				continue
			}
			for index, key := range keys {
				// Uses the shared prefix constant from pkg/config/model to stay in sync with
				// pkg/config/utils.IsDelaDirective without creating a setup <-> utils import cycle.
				if !strings.HasPrefix(strings.TrimSpace(key), pkgconfigmodel.DelaDirectivePrefix) {
					continue
				}
				addDelegatedAuthEndpointInstance(ctx, config, delegatedAuthComp, defaultProviderConfig, key,
					fmt.Sprintf("additional endpoint %q at %q", domain, configKey),
					delegatedauth.InstanceParams{
						APIKeyConfigKey:              fmt.Sprintf("%s[%s][%d]", configKey, domain, index),
						AdditionalEndpointDomain:     domain,
						AdditionalEndpointsConfigKey: configKey,
						AdditionalEndpointKeyIndex:   index,
						TargetSite:                   domain,
					})
			}
		}
	}
}

// addDelegatedAuthEndpointInstance parses a directive and registers its write-back target.
func addDelegatedAuthEndpointInstance(ctx context.Context, config pkgconfigmodel.Config, delegatedAuthComp delegatedauth.Component, defaultProviderConfig common.ProviderConfig, directiveText, describe string, params delegatedauth.InstanceParams) {
	directive, ok := parseDelaDirective(directiveText)
	if !ok {
		log.Warnf("Could not parse the delegated auth directive for %s; it will be ignored and no data will be sent to that endpoint", describe)
		return
	}
	instanceProviderConfig, err := providerConfigForDirective(directive, defaultProviderConfig)
	if err != nil {
		log.Errorf("Failed to configure delegated auth for %s: %v", describe, err)
		return
	}
	log.Infof("Configuring delegated authentication for %s", describe)
	if directive.params["fallback"] != "" {
		log.Warnf("The fallback parameter in the delegated auth directive for %s is not supported and will be ignored", describe)
	}

	params.Config = config
	params.OrgUUID = directive.orgUUID
	params.RefreshInterval = config.GetInt("delegated_auth.refresh_interval_mins")
	params.AllowAsyncStartup = true
	params.APIKeyConfigKey += "[" + directive.orgUUID + "]"
	params.ProviderConfig = instanceProviderConfig
	params.AdditionalEndpointDirective = directiveText

	err = addDelegatedAuthInstance(ctx, delegatedAuthComp, params)
	if err != nil {
		log.Errorf("Failed to configure delegated auth for %s: %v", describe, err)
	}
}

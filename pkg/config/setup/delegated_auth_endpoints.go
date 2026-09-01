// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v2"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// delaDirectiveRe matches a delegated-auth directive embedded as a value in `additional_endpoints`,
// e.g. "DELA(<org_uuid>, aws)" or "DELA(<org_uuid>, aws, region=us-east-1)".
var delaDirectiveRe = regexp.MustCompile(`^DELA\(\s*([^,]+?)\s*,\s*([^,)]+?)\s*(?:,\s*(.*))?\)$`)

// delaDirective is a parsed DELA(...) directive found in an `additional_endpoints` value.
type delaDirective struct {
	orgUUID  string
	provider string
	// params keys are lower-cased, so lookups here and the redaction regex below agree on which
	// spellings of a parameter name they cover.
	params map[string]string
}

// parseDelaDirective parses a DELA(<org_uuid>, <provider>[, key=value, ...]) directive.
// Returns ok=false for anything that isn't a well-formed directive.
func parseDelaDirective(value string) (delaDirective, bool) {
	matches := delaDirectiveRe.FindStringSubmatch(strings.TrimSpace(value))
	if matches == nil {
		return delaDirective{}, false
	}

	orgUUID := strings.TrimSpace(matches[1])
	provider := strings.TrimSpace(matches[2])
	if orgUUID == "" || provider == "" {
		return delaDirective{}, false
	}

	params := map[string]string{}
	if matches[3] != "" {
		for _, pair := range strings.Split(matches[3], ",") {
			kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
			if len(kv) != 2 {
				return delaDirective{}, false
			}
			key, val := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
			if key == "" || val == "" {
				return delaDirective{}, false
			}
			params[strings.ToLower(key)] = val
		}
	}

	return delaDirective{orgUUID: orgUUID, provider: strings.ToLower(provider), params: params}, true
}

// secretHandlePrefix marks a value the secrets backend is meant to resolve.
const secretHandlePrefix = "ENC["

// resolveFallbackAPIKey resolves a directive's fallback=<value> through the secrets backend when it
// is an ENC[...] handle. An unresolved handle is dropped rather than returned: shipping the literal
// "ENC[...]" text as an API key would authenticate nothing and leak the handle to the intake.
//
// TODO: The resolved value is a snapshot taken at config-load time. If the secrets backend rotates
// the handle later, this instance keeps the old value until the agent restarts or the config is
// reloaded. A future change should pass the raw handle and the secrets resolver to the component
// so it can re-resolve on secret refresh, or re-run this function when the secrets subscriber fires.
func resolveFallbackAPIKey(secretResolver secrets.Component, fallback string, origin string) string {
	if fallback == "" {
		return ""
	}
	if !strings.HasPrefix(strings.TrimSpace(fallback), secretHandlePrefix) {
		return fallback
	}
	if secretResolver == nil {
		log.Warnf("Delegated auth fallback key at %q is a secret handle but no secrets backend is configured; ignoring the fallback", origin)
		return ""
	}

	resolved, err := resolveThroughSecretsBackend(secretResolver, fallback, origin)
	if err != nil {
		log.Warnf("Failed to resolve the secret in the delegated auth fallback key at %q: %v; ignoring the fallback", origin, err)
		return ""
	}
	if strings.HasPrefix(strings.TrimSpace(resolved), secretHandlePrefix) {
		log.Warnf("Delegated auth fallback key at %q did not resolve to a secret value; ignoring the fallback", origin)
		return ""
	}
	return resolved
}

// resolveThroughSecretsBackend runs a single scalar through the resolver, which works on YAML
// documents rather than bare strings.
func resolveThroughSecretsBackend(secretResolver secrets.Component, value, origin string) (string, error) {
	wrapped, err := yaml.Marshal(map[string]string{"v": value})
	if err != nil {
		return "", err
	}
	resolved, err := secretResolver.Resolve(wrapped, origin, "", "", false)
	if err != nil {
		return "", err
	}
	var out map[string]string
	if err := yaml.Unmarshal(resolved, &out); err != nil {
		return "", err
	}
	return out["v"], nil
}

// fallbackParamRe matches a fallback=<value> parameter within a raw DELA(...) string for redaction.
// The (?i) matches parseDelaDirective's lower-casing of parameter names, so every spelling that
// parses as a fallback also redacts as one.
var fallbackParamRe = regexp.MustCompile(`(?i)(fallback\s*=\s*)[^,)]*`)

// redactDelaDirectiveForLogging masks any fallback=<key> parameter's value in value, for logging.
func redactDelaDirectiveForLogging(value string) string {
	return fallbackParamRe.ReplaceAllString(value, "${1}***")
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

// mapShapeDelegatedAuthEndpointKeys lists map-shaped settings that support DELA write-back.
var mapShapeDelegatedAuthEndpointKeys = []string{
	// Read by pkg/config/utils.GetMultipleEndpoints and served by comp/forwarder/defaultforwarder,
	// which covers metrics, events, service checks and everything else on the main forwarder.
	"additional_endpoints",
	// Read by comp/trace/config's appendEndpoints and served by the pkg/trace writers, which
	// cover traces and APM stats.
	"apm_config.additional_endpoints",
}

// listShapeDelegatedAuthEndpointKeys lists list-shaped settings that support DELA write-back.
var listShapeDelegatedAuthEndpointKeys = []string{
	// Read by comp/logs/agent/config and served by the logs HTTP destination.
	"logs_config.additional_endpoints",
}

// configureListShapeAdditionalEndpointsDelegatedAuth scans the list-shape additional_endpoints
// settings for DELA(...) directives in each entry's api_key and registers an instance per match.
func configureListShapeAdditionalEndpointsDelegatedAuth(ctx context.Context, config pkgconfigmodel.Config, delegatedAuthComp delegatedauth.Component, defaultProviderConfig common.ProviderConfig, secretResolver secrets.Component) {
	for _, configKey := range listShapeDelegatedAuthEndpointKeys {
		entries, _ := common.NormalizeListShapeEntries(config.Get(configKey))

		for index, rawEntry := range entries {
			// NormalizeListShapeEntries keeps non-map elements as-is - a bare string, or nil from a
			// blank YAML list item - so they round-trip unchanged. None can hold a directive.
			entry, ok := rawEntry.(map[string]any)
			if !ok {
				continue
			}

			apiKeyField, directive, ok := common.CaseInsensitiveStringFieldWithKey(entry, "api_key")
			if !ok || !strings.HasPrefix(strings.TrimSpace(directive), pkgconfigmodel.DelaDirectivePrefix) {
				continue
			}

			// The entry's host is both the destination the consumer looks the provider up by and
			// the site the auth proof is exchanged against. Without it the exchange would silently
			// fall back to the agent's primary site and fail for a different org.
			host, hasHost := common.CaseInsensitiveStringField(entry, "host")
			if !hasHost {
				log.Warnf("Additional endpoint entry %d at %q has a delegated auth directive but no host; it cannot be matched to a credential and will not send", index, configKey)
				continue
			}

			addDelegatedAuthEndpointInstance(ctx, config, delegatedAuthComp, defaultProviderConfig, secretResolver, directive,
				fmt.Sprintf("additional endpoint entry %d (%q) at %q", index, host, configKey),
				fmt.Sprintf("%s[%d]", configKey, index),
				configKey, host, append(strings.Split(configKey, "."), strconv.Itoa(index), apiKeyField))
		}
	}
}

// configureAdditionalEndpointsDelegatedAuth scans the supported additional_endpoints settings for
// DELA(...) directives and registers a delegated-auth instance for each one.
//
// Unlike the flat-key path, nothing is written back into the config: the directive stays where it
// is and the resolved credential reaches the consumer through the Provider returned by AddInstance.
// That keeps the credential out of the config tree, and means the consumer needs no notion of a
// key that has not arrived yet - it just cannot send until the Provider says it can.
func configureAdditionalEndpointsDelegatedAuth(ctx context.Context, config pkgconfigmodel.Config, delegatedAuthComp delegatedauth.Component, defaultProviderConfig common.ProviderConfig, secretResolver secrets.Component) {
	for _, configKey := range mapShapeDelegatedAuthEndpointKeys {
		for domain, keys := range config.GetStringMapStringSlice(configKey) {
			for index, key := range keys {
				// Uses the shared prefix constant from pkg/config/model to stay in sync with
				// pkg/config/utils.IsDelaDirective without creating a setup <-> utils import cycle.
				if !strings.HasPrefix(strings.TrimSpace(key), pkgconfigmodel.DelaDirectivePrefix) {
					continue
				}
				addDelegatedAuthEndpointInstance(ctx, config, delegatedAuthComp, defaultProviderConfig, secretResolver, key,
					fmt.Sprintf("additional endpoint %q at %q", domain, configKey),
					// The index disambiguates two directives under the same domain that share an
					// org UUID but differ in their params, e.g. two regions of the same org.
					fmt.Sprintf("%s[%s][%d]", configKey, domain, index),
					configKey, domain, append(strings.Split(configKey, "."), domain, strconv.Itoa(index)))
			}
		}
	}
}

// addDelegatedAuthEndpointInstance parses directiveText and registers the instance it describes,
// associating the resulting Provider with (configKey, destination) so the consumer can find it.
// describe names the endpoint in log messages; bookkeepingKey is a per-directive unique string.
func addDelegatedAuthEndpointInstance(ctx context.Context, config pkgconfigmodel.Config, delegatedAuthComp delegatedauth.Component, defaultProviderConfig common.ProviderConfig, secretResolver secrets.Component, directiveText, describe, bookkeepingKey, configKey, destination string, writebackPath []string) {
	directive, ok := parseDelaDirective(directiveText)
	if !ok {
		log.Warnf("Could not parse the delegated auth directive %q for %s; it will be ignored and no data will be sent to that endpoint", redactDelaDirectiveForLogging(directiveText), describe)
		return
	}
	instanceProviderConfig, err := providerConfigForDirective(directive, defaultProviderConfig)
	if err != nil {
		log.Errorf("Failed to configure delegated auth for %s: %v", describe, err)
		return
	}
	log.Infof("Configuring delegated authentication for %s", describe)

	// APIKeyConfigKey must be unique per directive because it keys the component's instances map.
	_, err = delegatedAuthComp.AddInstance(ctx, delegatedauth.InstanceParams{
		Config:          config,
		OrgUUID:         directive.orgUUID,
		RefreshInterval: config.GetInt("delegated_auth.refresh_interval_mins"),
		APIKeyConfigKey: bookkeepingKey + "[" + directive.orgUUID + "]",
		ProviderConfig:  instanceProviderConfig,
		ConfigKey:       configKey,
		Directive:       directiveText,
		TargetSite:      destination,
		FallbackAPIKey:  resolveFallbackAPIKey(secretResolver, directive.params["fallback"], configKey),
		WritebackPath:   writebackPath,
		Destination:     destination,
	})
	if err != nil {
		log.Errorf("Failed to configure delegated auth for %s: %v", describe, err)
	}
}

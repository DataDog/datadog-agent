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
	"time"

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

// delaDirective is a parsed DELA[...] directive found in an `additional_endpoints` value.
type delaDirective struct {
	orgUUID  string
	provider string
	// Parameter names are case-insensitive.
	params map[string]string
}

// parseDelaDirective parses DELA[<org_uuid>, <provider>[, <name>=<value>]...].
// Whitespace is allowed around separators. Supported parameters are region and fallback.
func parseDelaDirective(value string) (delaDirective, bool) {
	if strings.ContainsAny(value, "\r\n") {
		return delaDirective{}, false
	}
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, pkgconfigmodel.DelaDirectivePrefix) || !strings.HasSuffix(value, "]") {
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

// providerConfigForDirective builds a ProviderConfig for a DELA[...] directive, falling back to
// the process-wide default when the directive omits provider-specific overrides.
//
// Returns nil when neither the directive nor the default supplies provider-specific config (e.g.
// an AWS directive with no region and no process-wide delegated_auth.aws.region). A nil config
// tells the component to auto-detect, which is the right behavior for a directive that says only
// "DELA[org, aws]" — the region should come from the runtime environment, not default to empty.
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
		return nil, fmt.Errorf("unsupported provider %q in DELA[...] directive", directive.provider)
	}
}

// mapShapeDelegatedAuthEndpointKeys lists the map-shaped settings enabled for delegated-auth
// write-back. Keep this list small: each consumer must ignore DELA[...] until the key is written.
//
// Settings reached through pkg/config/utils.MakeEndpoints satisfy (a) already, because
// PartitionRealAndPendingKeys strips directives while keeping the domain alive.
var mapShapeDelegatedAuthEndpointKeys = []string{
	// Read by pkg/config/utils.GetMultipleEndpoints and served by comp/forwarder/defaultforwarder,
	// which covers metrics, events, service checks and everything else on the main forwarder.
	"additional_endpoints",
	// Read by pkg/process/runner/endpoint.GetAPIEndpoints and served by the process forwarders
	// (process, rt-process, connections). Pending directives keep the domain alive with no key
	// until the writeback lands. System-probe shares the helper but rejects pending directives
	// at startup via CheckAPIKeysResolved, since it cannot resolve them.
	"process_config.additional_endpoints",
}

// listShapeDelegatedAuthEndpointKeys lists the list-shaped settings enabled for write-back.
var listShapeDelegatedAuthEndpointKeys = []string{
	// Supported for HTTP logs endpoints. TCP endpoints skip pending directives; the HTTP
	// transport gate below is what decides whether the logs pipeline is on HTTP today.
	pkgconfigmodel.DelaLogsEndpointsKey,
	// Security-agent pipelines (CWS events, compliance events, activity-dump remote storage).
	// Their senders are the HTTP logs pipeline and never fall back to TCP, so they register
	// without a transport gate.
	pkgconfigmodel.DelaCWSEndpointsKey,
	pkgconfigmodel.DelaComplianceEndpointsKey,
	pkgconfigmodel.DelaActivityDumpEndpointsKey,
}

// listShapeKeyUsesHTTP reports whether the setting's consumer ships over HTTP today. Only the
// logs pipeline can fall back to TCP (which ignores delegated-auth entries); the security-agent
// pipelines are always HTTP.
func listShapeKeyUsesHTTP(config pkgconfigmodel.Reader, configKey string) bool {
	if configKey == pkgconfigmodel.DelaLogsEndpointsKey {
		return logsUseHTTPWithAdditionalEndpoints(config)
	}
	return true
}

// logsUseHTTPWithAdditionalEndpoints mirrors how the logs agent picks HTTP when additional endpoints
// are configured (BuildEndpointsWithConfig in comp/logs/agent/config): only when HTTP is forced, the
// logs URL is an HTTP proxy, or Observability Pipelines Worker is enabled. Otherwise it uses TCP,
// which ignores delegated-auth entries.
func logsUseHTTPWithAdditionalEndpoints(config pkgconfigmodel.Reader) bool {
	if config.GetBool("logs_config.use_http") || config.GetBool("logs_config.force_use_http") {
		return true
	}
	if url := config.GetString("logs_config.logs_dd_url"); strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return true
	}
	return config.GetBool("observability_pipelines_worker.logs.enabled") || config.GetBool("vector.logs.enabled")
}

// configureListShapeAdditionalEndpointsDelegatedAuth scans the list-shape additional_endpoints
// settings for DELA[...] directives in each entry's api_key and registers an instance per match.
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
			if !listShapeKeyUsesHTTP(config, configKey) {
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
// DELA[...] directives and registers a delegated-auth instance for each one.
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

	err = delegatedAuthComp.AddInstance(ctx, params)
	if err != nil {
		log.Errorf("Failed to configure delegated auth for %s: %v", describe, err)
	}
}

// defaultProviderConfigForConfig derives the process-wide delegated-auth provider configuration
// from the delegated_auth prefix: an explicit provider yields that provider's config, an empty
// one keeps nil so the component auto-detects.
func defaultProviderConfigForConfig(config pkgconfigmodel.Reader) common.ProviderConfig {
	switch config.GetString("delegated_auth.provider") {
	case "aws":
		return &cloudauthconfig.AWSProviderConfig{
			Region: config.GetString("delegated_auth.aws.region"),
		}
	default:
		// Empty provider means auto-detect, so ProviderConfig stays nil even when
		// delegated_auth.aws.region is set: a non-nil ProviderConfig means "explicitly configured"
		// downstream and would skip provider detection entirely. The component reads the configured
		// region itself once detection picks AWS.
		return nil
	}
}

// ConfigureDelegatedAuthEndpoints re-runs the additional_endpoints delegated-auth scan. Values
// merged after LoadDatadog (e.g. security-agent.yaml, fleet policies) are invisible to the scan
// that runs inside it; this covers them. Re-running on an entry whose directive was already
// resolved registers nothing: the scan re-reads the effective config, where the resolved key
// has replaced the directive text.
func ConfigureDelegatedAuthEndpoints(ctx context.Context, config pkgconfigmodel.Config, delegatedAuthComp delegatedauth.Component) {
	// Warn before registering: a synchronous resolution replaces the directive with the key,
	// which would hide it from the direct-send warning.
	WarnDirectSendDelegatedAuthDirectives(config)

	providerConfig := defaultProviderConfigForConfig(config)

	startupCtx := ctx
	cancelStartup := func() {}
	if timeout := config.GetInt("delegated_auth.startup_timeout_secs"); timeout > 0 {
		startupCtx, cancelStartup = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	}
	defer cancelStartup()

	configureAdditionalEndpointsDelegatedAuth(startupCtx, config, delegatedAuthComp, providerConfig)
	configureListShapeAdditionalEndpointsDelegatedAuth(startupCtx, config, delegatedAuthComp, providerConfig)
}

// directSendDelegatedAuthDirectiveKeys lists the list-shape settings whose payloads ship from
// the system-probe process and the config flag that moves them there; that process never runs
// the delegated-auth scan, so directives in those settings cannot resolve where the payload
// ships from.
var directSendDelegatedAuthDirectiveKeys = []struct {
	flag string
	key  string
}{
	{"runtime_security_config.direct_send_from_system_probe", pkgconfigmodel.DelaCWSEndpointsKey},
	{"runtime_security_config.direct_send_from_system_probe", pkgconfigmodel.DelaActivityDumpEndpointsKey},
	{"compliance_config.run_in_system_probe", pkgconfigmodel.DelaComplianceEndpointsKey},
}

type directSendDirective struct {
	// path is the config path of the entry holding the directive.
	path string
	// flag is the setting that moves the entry's payloads into the system-probe process.
	flag string
}

// directSendDelegatedAuthDirectives lists the additional-endpoint entries holding a DELA
// directive in settings that ship from the system-probe. The results feed a startup warning:
// those entries cannot resolve in the process that ships them.
func directSendDelegatedAuthDirectives(config pkgconfigmodel.Reader) []directSendDirective {
	var found []directSendDirective
	for _, setting := range directSendDelegatedAuthDirectiveKeys {
		if !config.GetBool(setting.flag) {
			continue
		}
		entries, _ := common.NormalizeListShapeEntries(config.Get(setting.key))
		for index, rawEntry := range entries {
			entry, ok := rawEntry.(map[string]any)
			if !ok {
				continue
			}
			if _, directive, ok := common.CaseInsensitiveStringFieldWithKey(entry, "api_key"); ok && strings.HasPrefix(strings.TrimSpace(directive), pkgconfigmodel.DelaDirectivePrefix) {
				found = append(found, directSendDirective{
					path: fmt.Sprintf("%s[%d]", setting.key, index),
					flag: setting.flag,
				})
			}
		}
	}
	return found
}

// WarnDirectSendDelegatedAuthDirectives warns when DELA directives sit in settings that ship
// from the system-probe: that process cannot resolve them, so those payloads will not dual-ship
// (and are held until restart). Called from the config load paths so that every placement of
// the directive (datadog.yaml, security-agent.yaml, fleet policies, system-probe.yaml) is
// flagged somewhere at startup.
func WarnDirectSendDelegatedAuthDirectives(config pkgconfigmodel.Reader) {
	for _, found := range directSendDelegatedAuthDirectives(config) {
		log.Warnf("%s uses delegated auth, but %s ships its payloads from the system-probe process, which cannot resolve delegated-auth directives; those payloads will not dual-ship", found.path, found.flag)
	}
}

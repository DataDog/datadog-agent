// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remoteconfig gives the autoscaling product its own Remote
// Configuration client, pointed at a Datadog endpoint other than the one the
// Cluster Agent uses for everything else. Internal capability for Datadog
// staging clusters: undocumented, empty by default, unsupported for customers.
package remoteconfig

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
)

const (
	// URLSetting is the Datadog endpoint autoscaling reads its configuration
	// from. A non-empty value is the only trigger: there is no enabled flag.
	URLSetting = "autoscaling.endpoint.url"
	// APIKeySetting is the API key for the organization serving URLSetting, kept
	// separate from the dualshipping additional-endpoint configuration.
	APIKeySetting = "autoscaling.endpoint.api_key"

	// The settings below mirror their remote_configuration counterparts, for a
	// backend whose trust material is not the one embedded for its site.

	// SiteSetting overrides the Datadog site derived from URLSetting, which
	// selects the embedded trusted roots.
	SiteSetting = "autoscaling.endpoint.site"
	// ConfigRootSetting overrides the trusted root metadata of the config repository.
	ConfigRootSetting = "autoscaling.endpoint.config_root"
	// DirectorRootSetting overrides the trusted root metadata of the director repository.
	DirectorRootSetting = "autoscaling.endpoint.director_root"
	// KeySetting is the legacy per-organization remote configuration key.
	KeySetting = "autoscaling.endpoint.key"

	// DatabaseFileName is the uptane cache for this client. It must differ from
	// the default service's, which lives in the same run_path.
	DatabaseFileName = "remote-config-autoscaling.db"
)

// Settings is a validated autoscaling configuration endpoint.
type Settings struct {
	// URL is the endpoint the client polls.
	URL string
	// APIKey authenticates against the organization serving URL.
	APIKey string
	// Site selects the trusted roots the client and service pin. It defaults to
	// the site derived from URL.
	Site string
	// ConfigRoot and DirectorRoot override the trusted root metadata embedded
	// for Site. Empty means use the embedded roots.
	ConfigRoot   string
	DirectorRoot string
	// Key is the legacy per-organization remote configuration key, usually empty.
	Key string
}

// IsConfigured reports whether an autoscaling configuration endpoint is set.
// When it is not, autoscaling uses the Cluster Agent's default client and
// nothing in this package applies.
func IsConfigured(cfg model.Reader) bool {
	return strings.TrimSpace(cfg.GetString(URLSetting)) != ""
}

// Load validates the autoscaling configuration endpoint settings. It errors
// rather than falling back to the default endpoint, which would quietly read
// recommendations from the wrong organization; the caller fails startup. The
// errors are logged, so they report the URL through displayURL.
func Load(cfg model.Reader) (Settings, error) {
	rawURL := strings.TrimSpace(cfg.GetString(URLSetting))
	if rawURL == "" {
		return Settings{}, fmt.Errorf("%s is not set", URLSetting)
	}

	site := configUtils.ExtractSiteFromURL(rawURL)
	if site == "" {
		return Settings{}, fmt.Errorf("%s (%q) does not point at a known Datadog site", URLSetting, displayURL(rawURL))
	}

	if override := strings.TrimSpace(cfg.GetString(SiteSetting)); override != "" {
		site = override
	}

	apiKey := configUtils.SanitizeAPIKey(cfg.GetString(APIKeySetting))
	if apiKey == "" {
		return Settings{}, fmt.Errorf("%s must be set when %s is set", APIKeySetting, URLSetting)
	}

	return Settings{
		URL:          rawURL,
		APIKey:       apiKey,
		Site:         site,
		ConfigRoot:   strings.TrimSpace(cfg.GetString(ConfigRootSetting)),
		DirectorRoot: strings.TrimSpace(cfg.GetString(DirectorRootSetting)),
		Key:          strings.TrimSpace(cfg.GetString(KeySetting)),
	}, nil
}

// displayURL keeps only the parts of a configured URL that identify the backend,
// for Load's errors and for anything that renders the setting. Those end up in
// logs and flares, so userinfo, query and fragment are dropped. It trims because
// a caller may not have been through Load, and url.Parse rejects a padded URL.
func displayURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		// Startup fails on an unparseable URL, so say nothing here rather than
		// echo a value that may carry a credential.
		return "<invalid>"
	}
	display := url.URL{Scheme: parsed.Scheme, Host: parsed.Host, Path: parsed.Path}
	return display.String()
}

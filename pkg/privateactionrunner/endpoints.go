// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"errors"
	"net/url"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	configconstants "github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
)

// Site returns the logical Datadog site independently of intake URL overrides.
func Site(cfg model.Reader) string {
	site := strings.TrimRight(strings.ToLower(strings.TrimSpace(cfg.GetString("site"))), ".")
	if site == "" {
		return configconstants.DefaultSite
	}
	return site
}

// APIBaseURL resolves the PAR API endpoint independently of the runner's identity.
func APIBaseURL(cfg model.Reader, site string) (string, error) {
	// Like RC, an explicit non-empty override is a full URL, not a site.
	if cfg.IsConfigured("remote_configuration.rc_dd_url") && cfg.GetString("remote_configuration.rc_dd_url") != "" {
		endpoint := cfg.GetString("remote_configuration.rc_dd_url")
		u, err := url.Parse(endpoint)
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return "", errors.New("invalid remote_configuration.rc_dd_url for PAR: expected an HTTP(S) URL with a host")
		}
		// Match the split-mode client's URL restrictions; never log credentials.
		if u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(endpoint, "#") {
			return "", errors.New("remote_configuration.rc_dd_url for PAR must not contain credentials, a query, or a fragment")
		}
		if u.Scheme != "https" && !cfg.GetBool("remote_configuration.no_tls") {
			return "", errors.New("remote_configuration.rc_dd_url requires HTTPS unless remote_configuration.no_tls is enabled")
		}
		if cfg.GetBool("skip_ssl_validation") && !cfg.GetBool("remote_configuration.no_tls_validation") {
			return "", errors.New("remote_configuration.rc_dd_url requires TLS verification unless remote_configuration.no_tls_validation is enabled")
		}
		return endpoint, nil
	}
	if os.Getenv(constants.InternalUseDDURLForOPMSEnvVar) == "true" {
		return strings.TrimSuffix(configutils.GetMainEndpoint(cfg, "https://api.", "dd_url"), "/"), nil
	}
	return "https://api." + site, nil
}

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

// APIBaseURL resolves the PAR API endpoint independently of the RC endpoint and runner identity.
func APIBaseURL(cfg model.Reader, site string) (string, error) {
	if endpoint := cfg.GetString(PARDDURL); endpoint != "" {
		u, err := url.Parse(endpoint)
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return "", errors.New("invalid private_action_runner.par_dd_url: expected an HTTP(S) URL with a host")
		}
		// Match the split-mode client's URL restrictions; never log credentials.
		if u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(endpoint, "#") {
			return "", errors.New("private_action_runner.par_dd_url must not contain credentials, a query, or a fragment")
		}
		return endpoint, nil
	}
	if os.Getenv(constants.InternalUseDDURLForOPMSEnvVar) == "true" {
		return strings.TrimSuffix(configutils.GetMainEndpoint(cfg, "https://api.", "dd_url"), "/"), nil
	}
	return "https://api." + site, nil
}

// ShouldWarnAboutRCOverride reports nonstandard RC routing without an explicit PAR URL.
func ShouldWarnAboutRCOverride(cfg model.Reader) bool {
	rcURL := cfg.GetString("remote_configuration.rc_dd_url")
	return cfg.GetBool(Enabled) && cfg.GetString(PARDDURL) == "" &&
		os.Getenv(constants.InternalUseDDURLForOPMSEnvVar) != "true" &&
		rcURL != "" && !isDefaultRCURL(rcURL, Site(cfg))
}

func isDefaultRCURL(raw, site string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil &&
		strings.TrimSuffix(strings.ToLower(u.Hostname()), ".") == "config."+site &&
		(u.Port() == "" || u.Port() == "443") && (u.Path == "" || u.Path == "/") &&
		u.RawQuery == "" && !u.ForceQuery && !strings.Contains(raw, "#")
}

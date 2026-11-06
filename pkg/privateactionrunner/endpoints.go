// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"errors"
	"net/url"
	"os"
	"strconv"
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
	if endpoint := cfg.GetString(RunnerURL); endpoint != "" {
		u, err := url.Parse(endpoint)
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return "", errors.New("invalid private_action_runner.par_dd_url: expected an HTTP(S) URL with a host")
		}
		if port := u.Port(); port != "" {
			if _, err := strconv.ParseUint(port, 10, 16); err != nil {
				return "", errors.New("private_action_runner.par_dd_url port must be between 0 and 65535")
			}
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

// ShouldWarnAboutRCOverride reports an RC override without an explicit PAR URL when PAR is enabled.
func ShouldWarnAboutRCOverride(cfg model.Reader) bool {
	return cfg.GetBool(Enabled) &&
		cfg.GetString(RemoteConfigURL) != "" &&
		cfg.GetString(RunnerURL) == ""
}

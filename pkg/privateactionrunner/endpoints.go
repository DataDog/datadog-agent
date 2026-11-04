// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
)

// Site returns the logical Datadog site, retaining the Agent's dd_url precedence.
// An unrecognized endpoint has no site; callers retain their existing fallback policy.
func Site(cfg model.Reader) string {
	return configutils.ExtractSiteFromURL(configutils.GetMainEndpoint(cfg, "https://api.", "dd_url"))
}

// APIBaseURL resolves the PAR API endpoint independently of the runner's identity.
func APIBaseURL(cfg model.Reader, site string) string {
	if os.Getenv(constants.InternalUseDDURLForOPMSEnvVar) == "true" {
		return strings.TrimSuffix(configutils.GetMainEndpoint(cfg, "https://api.", "dd_url"), "/")
	}
	return "https://api." + site
}

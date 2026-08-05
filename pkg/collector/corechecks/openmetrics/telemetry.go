// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import (
	"strings"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

const (
	configureOutcomeLoaded   = "loaded"
	configureOutcomeFallback = "fallback"
	configureOutcomeError    = "error"

	configureReasonNone              = "none"
	configureReasonUnsupportedAuth   = "unsupported_auth_type"
	configureReasonLegacyAuth        = "use_legacy_auth_encoding"
	configureReasonUnsupportedConfig = "unsupported_config"
	configureReasonParseConfig       = "parse_config"
	configureReasonNewScraper        = "new_scraper"
)

var configureTelemetry = telemetryimpl.GetCompatComponent().NewCounter(
	"openmetrics_core",
	"configure",
	[]string{"outcome", "reason"},
	"OpenMetrics core check configure attempts by outcome.",
)

func recordConfigureTelemetry(outcome, reason string) {
	configureTelemetry.Inc(outcome, reason)
}

func unsupportedConfigTelemetryReason(err error) string {
	errText := err.Error()
	switch {
	case strings.Contains(errText, "auth_type"):
		return configureReasonUnsupportedAuth
	case strings.Contains(errText, "use_legacy_auth_encoding"):
		return configureReasonLegacyAuth
	default:
		return configureReasonUnsupportedConfig
	}
}

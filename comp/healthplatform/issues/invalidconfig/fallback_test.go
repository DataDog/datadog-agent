// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package invalidconfig

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/require"
)

func TestFallbackIssue(t *testing.T) {
	fallback := model.ConfigFallback{Key: "system_probe_config.max_tracked_connections", Reason: "must be positive", DefaultValue: 65536}
	issue, err := BuildFallbackIssue("system-probe", "/etc/datadog-agent/system-probe.yaml", []model.ConfigFallback{fallback, fallback})
	require.NoError(t, err)
	require.Empty(t, issue.Id)
	require.Equal(t, SystemProbeFallbackIssueName, issue.IssueName)
	require.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.Severity)
	require.Equal(t, "`/system_probe_config/max_tracked_connections` must be positive. System-probe is using the default value of `65536`.", issue.Description)
	require.Len(t, issue.Extra.Fields["fallbacks"].GetListValue().Values, 1)
	require.Contains(t, issue.Remediation.Steps[1].Text, "remove the setting")
	fallback.DefaultValue = nil
	fallback.ReplacementSetting = "system_probe_config.max_tracked_connections"
	fallback.Key = "system_probe_config.max_closed_connections_buffered"
	issue, err = BuildFallbackIssue("system-probe", "", []model.ConfigFallback{fallback})
	require.NoError(t, err)
	encoded, err := json.Marshal(issue)
	require.NoError(t, err)
	require.Contains(t, issue.Description, "using a replacement based on `/system_probe_config/max_tracked_connections`")
	require.NotContains(t, string(encoded), "default_value")
	require.NotContains(t, issue.Description, "unknown path")
}

func TestFallbackSuppressesConversionForSameSetting(t *testing.T) {
	conversions := []model.ConfigTypeConversion{{Key: "min_tls_version"}, {Key: "dogstatsd_port"}}
	filtered := FilterConfigConversions(conversions, []model.ConfigFallback{{Key: "min_tls_version"}})
	require.Equal(t, []model.ConfigTypeConversion{{Key: "dogstatsd_port"}}, filtered)
}

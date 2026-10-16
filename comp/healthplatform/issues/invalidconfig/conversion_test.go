// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package invalidconfig

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestConversionIssue(t *testing.T) {
	for _, component := range []string{"agent", "system-probe"} {
		t.Run(component, func(t *testing.T) {
			issue, err := BuildConversionIssue(component, "/etc/datadog-agent/datadog.yaml", []model.ConfigTypeConversion{
				{Key: "dogstatsd_port", Source: model.SourceFile, FromType: "string", ToType: "integer"},
			})
			require.NoError(t, err)
			require.Empty(t, issue.Id)
			if component == "agent" {
				require.Equal(t, ConversionIssueName, issue.IssueName)
			} else {
				require.Equal(t, SystemProbeConversionIssueName, issue.IssueName)
			}
			require.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_LOW, issue.Severity)
			require.Contains(t, issue.Description, "`/dogstatsd_port` was provided as a string.")
			require.Contains(t, issue.Description, "converted it to a whole number while loading the configuration.")
			require.Contains(t, issue.Remediation.Steps[1].Text, "whole number without quotation marks")
			require.Contains(t, issue.Remediation.Steps[0].Text, "/etc/datadog-agent/datadog.yaml")
		})
	}
	issue, err := BuildConversionIssue("agent", "", []model.ConfigTypeConversion{
		{Key: "endpoints", Path: "/https:~1~1user:secret-sentinel@example.com", FromType: "integer", ToType: "string"},
	})
	require.NoError(t, err)
	encoded, err := json.Marshal(issue)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "secret-sentinel")
	require.NotContains(t, string(encoded), "unknown path")
	_, err = BuildConversionIssue("agent", "", nil)
	require.Error(t, err)
}

func TestConversionCheck(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `dogstatsd_port: "9000"`)
	cfg.SetInTest("health_platform.invalidconfig_check.enabled", true)
	hostname, _ := hostnamemock.NewMock("test-host")
	m := newConversionModule(issues.ModuleDeps{Config: cfg, Hostname: hostname})
	check := m.BuiltInPeriodicHealthCheck().Fn
	reports, err := check()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	issue, err := m.BuildIssue(reports[0].Context)
	require.NoError(t, err)
	require.Contains(t, issue.Description, "/dogstatsd_port")
	require.NotContains(t, issue.Description, "9000")
	cfg.Set("dogstatsd_port", 9000, model.SourceFile)
	reports, err = check()
	require.NoError(t, err)
	require.Empty(t, reports)
}

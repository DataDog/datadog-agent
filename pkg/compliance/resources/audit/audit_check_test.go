// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package audit

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/datadog-agent/pkg/compliance/rego"
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/constants"

	"github.com/elastic/go-libaudit/rule"
	"github.com/stretchr/testify/mock"
	assert "github.com/stretchr/testify/require"
)

type setupEnvFunc func(t *testing.T, env *mocks.Env)

func TestAuditCheck(t *testing.T) {

	tests := []struct {
		name         string
		rules        []*rule.FileWatchRule
		resource     compliance.RegoInput
		hostPath     string
		setup        setupEnvFunc
		expectReport *compliance.Report
	}{
		/*
			{
				name:  "no file rules",
				rules: []*rule.FileWatchRule{},
				resource: compliance.RegoInput{
					ResourceCommon: compliance.ResourceCommon{
						Audit: &compliance.Audit{
							Path: "/etc/docker/daemon.json",
						},
					},
				},
				hostPath: "./testdata/daemon.json",
				expectReport: &compliance.Report{
					Passed: false,
					Data:   event.Data{},
				},
			},
		*/
		{
			name: "file rule present",
			rules: []*rule.FileWatchRule{
				{
					Type: rule.FileWatchRuleType,
					Path: "/etc/docker/daemon.json",
					Permissions: []rule.AccessType{
						rule.ReadAccessType,
						rule.WriteAccessType,
						rule.AttributeChangeAccessType,
					},
				},
			},
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					Audit: &compliance.Audit{
						Path: "/etc/docker/daemon.json",
					},
				},
			},
			hostPath: "./testdata/daemon.json",
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"audit.enabled":     true,
					"audit.path":        "/etc/docker/daemon.json",
					"audit.permissions": "rwa",
				},
			},
		},
		{
			name: "file missing on the host",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					Audit: &compliance.Audit{
						Path: "/etc/docker/daemon.json",
					},
				},
			},
			hostPath: "./missing-file.json",
			expectReport: &compliance.Report{
				Passed: false,
				Error:  errors.New("rule-id: audit resource path does not exist"),
			},
		},
		{
			name: "file rule present (resolve path)",
			rules: []*rule.FileWatchRule{
				{
					Type: rule.FileWatchRuleType,
					Path: "/etc/docker/daemon.json",
					Permissions: []rule.AccessType{
						rule.ReadAccessType,
						rule.WriteAccessType,
					},
				},
			},
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					Audit: &compliance.Audit{
						Path: `process.flag("dockerd", "--config-file")`,
					},
				},
			},
			setup: func(t *testing.T, env *mocks.Env) {
				env.On("EvaluateFromCache", mock.Anything).Return("/etc/docker/daemon.json", nil)
			},
			hostPath: "./testdata/daemon.json",
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"audit.enabled":     true,
					"audit.path":        "/etc/docker/daemon.json",
					"audit.permissions": "rw",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			client := &mocks.AuditClient{}
			defer client.AssertExpectations(t)

			env := &mocks.Env{}
			defer env.AssertExpectations(t)

			env.On("MaxEventsPerRun").Return(30).Maybe()
			env.On("AuditClient").Return(client)
			env.On("ProvidedInput", "rule-id").Return(nil).Maybe()
			env.On("DumpInputPath").Return("").Maybe()
			env.On("ShouldSkipRegoEval").Return(false).Maybe()
			env.On("Hostname").Return("test-host").Maybe()
			env.On("NormalizeToHostRoot", mock.AnythingOfType("string")).Return(test.hostPath)
			if test.expectReport.Error == nil {
				client.On("GetFileWatchRules").Return(test.rules, nil)
			}

			if test.setup != nil {
				test.setup(t, env)
			}

			regoRule := &compliance.RegoRule{
				RuleCommon: compliance.RuleCommon{
					ID: "rule-id",
				},
				Inputs: []compliance.RegoInput{
					test.resource,
					{
						ResourceCommon: compliance.ResourceCommon{
							Constants: &compliance.ConstantsResource{
								Values: map[string]interface{}{
									"resource_type": "docker_daemon",
								},
							},
						},
					},
				},
				Imports: []string{
					"../../rego/rego_helpers/helpers.rego",
				},
				Module: `package datadog

import data.datadog as dd
import data.helpers as h

enabled_audits = [audit | audit := input.audit[_]; audit.enabled]

findings[f] {
        count(enabled_audits) > 0
        f := dd.passed_finding(
				h.resource_type,
                h.resource_id,
				h.audit_data(input.audit[0]),
        )
}

findings[f] {
        h.has_key(input, "audit")
        count(enabled_audits) == 0
        f := dd.failing_finding(
                h.resource_type,
                h.resource_id,
                { "error": input },
        )
}

findings[f] {
        not h.has_key(input, "audit")
        f := dd.error_finding(
                h.resource_type,
                h.resource_id,
                sprintf("%s: audit resource path does not exist", [input.context.ruleID]),
        )
}
`,
			}
			auditCheck := rego.NewCheck(regoRule)
			err := auditCheck.CompileRule(regoRule, "", &compliance.SuiteMeta{}, nil)
			assert.NoError(err)

			result := auditCheck.Check(env)
			assert.NotEmpty(result)

			jsonResult, _ := json.MarshalIndent(result, "", "  ")
			t.Log(string(jsonResult))

			assert.Equal(test.expectReport.Passed, result[0].Passed)
			assert.Equal(test.expectReport.Data, result[0].Data)
			if test.expectReport.Error != nil {
				assert.EqualError(test.expectReport.Error, result[0].Error.Error())
			}
		})
	}
}

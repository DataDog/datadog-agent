// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/elastic/go-libaudit/rule"

	"github.com/stretchr/testify/mock"
	assert "github.com/stretchr/testify/require"
)

type setupEnvFunc func(t *testing.T, env *mocks.Env)

func TestAuditCheck(t *testing.T) {

	tests := []struct {
		name         string
		rules        []*rule.FileWatchRule
		resource     compliance.Resource
		hostPath     string
		setup        setupEnvFunc
		expectReport *compliance.Report
	}{
		{
			name:  "no file rules",
			rules: []*rule.FileWatchRule{},
			resource: compliance.Resource{
				ResourceCommon: compliance.ResourceCommon{
					Audit: &compliance.Audit{
						Path: "/etc/docker/daemon.json",
					},
				},
				Condition: "audit.enabled",
			},
			hostPath: "./testdata/file/daemon.json",
			expectReport: &compliance.Report{
				Passed: false,
			},
		},
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
			resource: compliance.Resource{
				ResourceCommon: compliance.ResourceCommon{
					Audit: &compliance.Audit{
						Path: "/etc/docker/daemon.json",
					},
				},
				Condition: `audit.enabled && audit.permissions =~ "w"`,
			},
			hostPath: "./testdata/file/daemon.json",
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
			resource: compliance.Resource{
				ResourceCommon: compliance.ResourceCommon{
					Audit: &compliance.Audit{
						Path: "/etc/docker/daemon.json",
					},
				},
				Condition: `audit.enabled && audit.permissions =~ "w"`,
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
			resource: compliance.Resource{
				ResourceCommon: compliance.ResourceCommon{
					Audit: &compliance.Audit{
						Path: `process.flag("docker", "--config-file")`,
					},
				},
				Condition: `audit.enabled && audit.permissions =~ "r"`,
			},
			setup: func(t *testing.T, env *mocks.Env) {
				env.On("EvaluateFromCache", mock.Anything).Return("/etc/docker/daemon.json", nil)
			},
			hostPath: "./testdata/file/daemon.json",
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

			if test.rules != nil {
				client.On("GetFileWatchRules").Return(test.rules, nil)
			}

			env := &mocks.Env{}
			defer env.AssertExpectations(t)

			env.On("MaxEventsPerRun").Return(30).Maybe()
			env.On("AuditClient").Return(client)

			env.On("NormalizeToHostRoot", mock.AnythingOfType("string")).Return(test.hostPath)

			if test.setup != nil {
				test.setup(t, env)
			}

			auditCheck, err := newResourceCheck(env, "rule-id", test.resource)
			assert.NoError(err)

			result := auditCheck.check(env)

			assert.Equal(test.expectReport.Passed, result[0].Passed)
			assert.Equal(test.expectReport.Data, result[0].Data)
			if test.expectReport.Error != nil {
				assert.EqualError(test.expectReport.Error, result[0].Error.Error())
			}
		})
	}
}

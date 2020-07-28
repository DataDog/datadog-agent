// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
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
		setup        setupEnvFunc
		expectReport *report
		expectError  error
	}{
		{
			name:  "no file rules",
			rules: nil,
			resource: compliance.Resource{
				Audit: &compliance.Audit{
					Path: "/etc/docker/daemon.json",
				},
				Condition: "audit.enabled",
			},
			expectReport: &report{
				passed: false,
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
				Audit: &compliance.Audit{
					Path: "/etc/docker/daemon.json",
				},
				Condition: `audit.enabled && audit.permissions =~ "w"`,
			},
			expectReport: &report{
				passed: true,
				data: event.Data{
					"audit.enabled":     true,
					"audit.path":        "/etc/docker/daemon.json",
					"audit.permissions": "rwa",
				},
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
				Audit: &compliance.Audit{
					Path: `process.flag("docker", "--config-file")`,
				},
				Condition: `audit.enabled && audit.permissions =~ "r"`,
			},
			setup: func(t *testing.T, env *mocks.Env) {
				env.On("EvaluateFromCache", mock.Anything).Return("/etc/docker/daemon.json", nil)
			},
			expectReport: &report{
				passed: true,
				data: event.Data{
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

			client.On("GetFileWatchRules").Return(test.rules, nil)

			env := &mocks.Env{}
			defer env.AssertExpectations(t)
			env.On("AuditClient").Return(client)

			if test.setup != nil {
				test.setup(t, env)
			}

			expr, err := eval.ParseIterable(test.resource.Condition)
			assert.NoError(err)

			result, err := checkAudit(env, "rule-id", test.resource, expr)

			assert.Equal(test.expectError, err)
			assert.Equal(test.expectReport, result)
		})
	}
}

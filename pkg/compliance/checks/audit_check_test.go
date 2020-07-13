// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/elastic/go-libaudit/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestAuditCheck(t *testing.T) {
	type setupFunc func(t *testing.T, env *mocks.Env)
	type validateFunc func(t *testing.T, kv event.Data)

	tests := []struct {
		name     string
		rules    []*rule.FileWatchRule
		audit    *compliance.Audit
		setup    setupFunc
		validate validateFunc
	}{
		{
			name:  "no file rules",
			rules: nil,
			audit: &compliance.Audit{
				Path: "/etc/docker/daemon.json",
				Report: []compliance.ReportedField{
					{
						Property: "enabled",
						Kind:     compliance.PropertyKindAttribute,
					},
					{
						Property: "path",
						Kind:     compliance.PropertyKindAttribute,
					},
				},
			},
			validate: func(t *testing.T, kv event.Data) {
				assert.Equal(t,
					event.Data{
						"enabled": "false",
						"path":    "/etc/docker/daemon.json",
					},
					kv,
				)
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
			audit: &compliance.Audit{
				Path: "/etc/docker/daemon.json",
				Report: []compliance.ReportedField{
					{
						Property: "enabled",
						Kind:     compliance.PropertyKindAttribute,
					},
					{
						Property: "path",
						Kind:     compliance.PropertyKindAttribute,
					},
					{
						Property: "permissions",
						Kind:     compliance.PropertyKindAttribute,
					},
				},
			},
			validate: func(t *testing.T, kv event.Data) {
				assert.Equal(t,
					event.Data{
						"enabled":     "true",
						"path":        "/etc/docker/daemon.json",
						"permissions": "rwa",
					},
					kv,
				)
			},
		},
		{
			name: "file rule present (pathFrom)",
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
			audit: &compliance.Audit{
				PathFrom: compliance.ValueFrom{
					{
						Process: &compliance.ValueFromProcess{
							Name: "dockerd",
							Flag: "--config-file",
						},
					},
				},
				Report: []compliance.ReportedField{
					{
						Property: "enabled",
						Kind:     compliance.PropertyKindAttribute,
					},
					{
						Property: "path",
						Kind:     compliance.PropertyKindAttribute,
					},
					{
						Property: "permissions",
						Kind:     compliance.PropertyKindAttribute,
					},
				},
			},
			setup: func(t *testing.T, env *mocks.Env) {
				env.On("ResolveValueFrom", mock.AnythingOfType("compliance.ValueFrom")).Return("/etc/docker/daemon.json", nil)
			},
			validate: func(t *testing.T, kv event.Data) {
				assert.Equal(t,
					event.Data{
						"enabled":     "true",
						"path":        "/etc/docker/daemon.json",
						"permissions": "rwa",
					},
					kv,
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			reporter := &mocks.Reporter{}
			defer reporter.AssertExpectations(t)

			client := &mocks.AuditClient{}
			defer client.AssertExpectations(t)

			client.On("GetFileWatchRules").Return(test.rules, nil)

			env := &mocks.Env{}
			defer env.AssertExpectations(t)
			env.On("Reporter").Return(reporter)
			env.On("AuditClient").Return(client)

			if test.setup != nil {
				test.setup(t, env)
			}

			base := newTestBaseCheck(env, checkKindAudit)
			check, err := newAuditCheck(base, test.audit)
			assert.NoError(err)

			reporter.On(
				"Report",
				mock.AnythingOfType("*event.Event"),
			).Run(func(args mock.Arguments) {
				event := args.Get(0).(*event.Event)
				test.validate(t, event.Data)
			})

			err = check.Run()
			assert.NoError(err)
		})
	}
}

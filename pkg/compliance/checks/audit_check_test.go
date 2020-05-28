// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/elastic/go-libaudit/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestAuditCheck(t *testing.T) {
	type validateFunc func(t *testing.T, kv compliance.KV)

	tests := []struct {
		name     string
		rules    []*rule.FileWatchRule
		audit    *compliance.Audit
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
			validate: func(t *testing.T, kv compliance.KV) {
				assert.Equal(t,
					compliance.KV{
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
			validate: func(t *testing.T, kv compliance.KV) {
				assert.Equal(t,
					compliance.KV{
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

			reporter := &compliance.MockReporter{}
			defer reporter.AssertExpectations(t)

			client := &MockAuditClient{}
			defer client.AssertExpectations(t)

			client.On("GetFileWatchRules").Return(test.rules, nil)

			base := newTestBaseCheck(reporter)
			check, err := newAuditCheck(base, client, test.audit)
			assert.NoError(err)

			reporter.On(
				"Report",
				mock.AnythingOfType("*compliance.RuleEvent"),
			).Run(func(args mock.Arguments) {
				event := args.Get(0).(*compliance.RuleEvent)
				test.validate(t, event.Data)
			})

			err = check.Run()
			assert.NoError(err)
		})
	}
}

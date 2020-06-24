// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGroupCheck(t *testing.T) {
	type validateFunc func(t *testing.T, kv compliance.KVMap)

	tests := []struct {
		name         string
		etcGroupFile string
		group        *compliance.Group
		validate     validateFunc
	}{
		{
			name:         "docker group",
			etcGroupFile: "./testdata/group/etc-group",
			group: &compliance.Group{
				Name: "docker",
				Report: []compliance.ReportedField{
					{
						Property: "users",
						Kind:     compliance.PropertyKindAttribute,
					},
					{
						Property: "gid",
						Kind:     compliance.PropertyKindAttribute,
					},
				},
			},
			validate: func(t *testing.T, kv compliance.KVMap) {
				assert.Equal(t,
					compliance.KVMap{
						"gid":   "412",
						"users": "alice,bob,carlos,dan,eve",
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

			base := newTestBaseCheck(reporter, checkKindAudit)
			check, err := newGroupCheck(base, test.etcGroupFile, test.group)
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

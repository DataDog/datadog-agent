// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchScopeExpression(t *testing.T) {
	type test struct {
		name             string
		policyID         string
		scopeExpressions []scopeExpression
		env              map[string]interface{}
		match            bool
		err              bool
	}

	tests := []test{
		{
			name:     "Match scope expression",
			policyID: "policy1",
			scopeExpressions: []scopeExpression{
				{
					Expression: "'env:test' in tags",
					PolicyID:   "policy1",
				},
			},
			env: map[string]interface{}{
				"tags": []string{"env:test"},
			},
			match: true,
			err:   false,
		},
		{
			name: "Match true",
			scopeExpressions: []scopeExpression{
				{
					Expression: "true",
					PolicyID:   "policy1",
				},
			},
			policyID: "policy1",
			env:      nil,
			match:    true,
			err:      false,
		},
		{
			name:             "Policy not present",
			policyID:         "policy2",
			scopeExpressions: []scopeExpression{},
			env: map[string]interface{}{
				"tags": []string{"env:test"},
			},
			match: false,
			err:   true,
		},
		{
			name:     "Policy not matching",
			policyID: "policy1",
			scopeExpressions: []scopeExpression{
				{
					Expression: "'foo:bar' in tags",
					PolicyID:   "policy1",
				},
			},
			env: map[string]interface{}{
				"tags": []string{"env:test"},
			},
			match: false,
			err:   false,
		},
		{
			name:     "Multiple policies -- one matching",
			policyID: "policy1",
			scopeExpressions: []scopeExpression{
				{
					Expression: "'env:test' in tags",
					PolicyID:   "policy1",
				},
				{
					Expression: "'foo:bar' in tags",
					PolicyID:   "policy2",
				},
			},
			env: map[string]interface{}{
				"tags": []string{"env:test"},
			},
			match: true,
			err:   false,
		},
		{
			name:     "Multiple tags in expression",
			policyID: "policy1",
			scopeExpressions: []scopeExpression{
				{
					Expression: "any(['env:test', 'env:prod'], {# in tags})",
					PolicyID:   "policy1",
				},
			},
			env: map[string]interface{}{
				"tags": []string{"env:test", "foo:bar"},
			},
			match: true,
			err:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			orderConfig := orderConfig{
				ScopeExpressions: test.scopeExpressions,
			}

			match, err := orderConfig.Match(test.policyID, test.env)
			assert.Equal(t, err != nil, test.err)
			assert.Equal(t, test.match, match)
		})
	}
}

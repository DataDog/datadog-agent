// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test && pcap && cgo

package rules

import (
	"errors"
	"testing"

	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNetworkFilterActionWithInvalidFilter ensures that when a policy contains a rule
// with an invalid BPF expression alongside a rule with a valid one, the rule checker
// reports a load error for the invalid rule while the valid rule is accepted.
func TestNetworkFilterActionWithInvalidFilter(t *testing.T) {
	invalidFilter := "((( invalid bpf syntax"
	validFilter := "tcp dst port 5555 and tcp[tcpflags] == tcp-syn"

	t.Run("precheck", func(t *testing.T) {
		err := (&NetworkFilterDefinition{
			BPFFilter: invalidFilter,
			Policy:    "drop",
			Scope:     "cgroup",
		}).PreCheck(PolicyLoaderOpts{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "valid BPF filter")

		err = (&NetworkFilterDefinition{
			BPFFilter: validFilter,
			Policy:    "drop",
			Scope:     "cgroup",
		}).PreCheck(PolicyLoaderOpts{})
		assert.NoError(t, err)
	})

	t.Run("rule_load", func(t *testing.T) {
		rs := newRuleSet()

		policyRules := []*PolicyRule{
			{
				Def: &RuleDefinition{
					ID:         "invalid",
					Expression: `exec.file.name == "test"`,
					Actions: []*ActionDefinition{
						{
							NetworkFilter: &NetworkFilterDefinition{
								BPFFilter: invalidFilter,
								Policy:    "drop",
								Scope:     "cgroup",
							},
						},
					},
				},
			},
			{
				Def: &RuleDefinition{
					ID:         "valid",
					Expression: `exec.file.name == "test"`,
					Actions: []*ActionDefinition{
						{
							NetworkFilter: &NetworkFilterDefinition{
								BPFFilter: validFilter,
								Policy:    "drop",
								Scope:     "cgroup",
							},
						},
					},
				},
			},
		}

		errs := rs.PopulateFieldsWithRuleActionsData(policyRules, PolicyLoaderOpts{})
		require.NotNil(t, errs)
		require.Error(t, errs.ErrorOrNil())

		ruleErrors := ruleLoadErrors(errs)
		require.Len(t, ruleErrors, 1)
		assert.Equal(t, "invalid", ruleErrors[0].Rule.Def.ID)
		assert.Contains(t, ruleErrors[0].Error(), "valid BPF filter")
	})
}

func ruleLoadErrors(err *multierror.Error) []*ErrRuleLoad {
	var out []*ErrRuleLoad
	if err == nil {
		return out
	}

	for _, e := range err.Errors {
		var ruleErr *ErrRuleLoad
		if errors.As(e, &ruleErr) {
			out = append(out, ruleErr)
		}
	}

	return out
}

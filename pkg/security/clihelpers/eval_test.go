// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package clihelpers

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/hashicorp/go-multierror"
)

type fakeProvider struct {
	data []byte
}

func (p *fakeProvider) LoadPolicies(macroFilters []rules.MacroFilter, ruleFilters []rules.RuleFilter) ([]*rules.Policy, *multierror.Error) {
	pInfo := &rules.PolicyInfo{
		Name:         "default.policy",
		Source:       "fake",
		InternalType: "fake",
	}

	var (
		errs     *multierror.Error
		policies []*rules.Policy
	)

	reader := bytes.NewReader(p.data)
	policy, err := rules.LoadPolicy(pInfo, reader, macroFilters, ruleFilters)
	if err != nil {
		errs = multierror.Append(errs, err)
	} else {
		policies = append(policies, policy)
	}

	return policies, errs
}

func (p *fakeProvider) SetOnNewPoliciesReadyCb(_ func(silent bool)) {}

func (p *fakeProvider) Start() {}

func (p *fakeProvider) Close() error {
	return nil
}

func (p *fakeProvider) Type() string {
	return "fake"
}

func TestParseVariableEntry(t *testing.T) {
	t.Run("old format direct value", func(t *testing.T) {
		v, opts, err := parseVariableEntry("hello")
		if err != nil {
			t.Fatal(err)
		}
		if v != "hello" {
			t.Fatalf("expected 'hello', got %v", v)
		}
		if opts.Private {
			t.Fatal("expected private=false")
		}
	})

	t.Run("new format value only", func(t *testing.T) {
		v, opts, err := parseVariableEntry(map[string]any{"value": "hello"})
		if err != nil {
			t.Fatal(err)
		}
		if v != "hello" {
			t.Fatalf("expected 'hello', got %v", v)
		}
		if opts.Private {
			t.Fatal("expected private=false")
		}
	})

	t.Run("new format with private", func(t *testing.T) {
		v, opts, err := parseVariableEntry(map[string]any{"value": "secret", "private": true})
		if err != nil {
			t.Fatal(err)
		}
		if v != "secret" {
			t.Fatalf("expected 'secret', got %v", v)
		}
		if !opts.Private {
			t.Fatal("expected private=true")
		}
	})

	t.Run("new format missing value", func(t *testing.T) {
		_, _, err := parseVariableEntry(map[string]any{"private": true})
		if err == nil {
			t.Fatal("expected error for missing 'value' field")
		}
	})
}

func TestEvalRule(t *testing.T) {
	var policy = `
rules:
  - id: IMDS
    expression: imds.cloud_provider == "aws" && process.file.name not in ${imds_v1_usage_services}
    actions:
      - set:
          name: imds_v1_usage_services
          field: process.file.name
          ttl: 10000000000
          append: true
`

	provider := &fakeProvider{
		data: []byte(policy),
	}

	t.Run("ok", func(t *testing.T) {
		var testData = `
		{
			"type": "imds",
			"values": {
				"imds.cloud_provider": "aws",
				"process.file.name": "curl"
			},
			"variables": {
				"imds_v1_usage_services": ["curl"]
			}
		}
	`

		decoder := json.NewDecoder(bytes.NewBufferString(testData))

		report, err := evalRule(provider, decoder, EvalRuleParams{})
		if err != nil {
			t.Fatalf("error evaluating rule: %s", err)
		}

		if report.Succeeded {
			t.Fatalf("expected rule to fail")
		}
	})

	t.Run("ko", func(t *testing.T) {
		var testData = `
		{
			"type": "imds",
			"values": {
				"imds.cloud_provider": "aws",
				"process.file.name": "curl"
			},
			"variables": {
				"imds_v1_usage_services": ["wget"]
			}
		}
	`

		decoder := json.NewDecoder(bytes.NewBufferString(testData))

		report, err := evalRule(provider, decoder, EvalRuleParams{})
		if err != nil {
			t.Fatalf("error evaluating rule: %s", err)
		}

		if report.Succeeded {
			t.Fatalf("expected rule to fail")
		}
	})

	t.Run("new variable format with private", func(t *testing.T) {
		var testData = `
		{
			"type": "imds",
			"values": {
				"imds.cloud_provider": "aws",
				"process.file.name": "curl"
			},
			"variables": {
				"imds_v1_usage_services": {
					"value": ["curl"],
					"private": true
				}
			}
		}
	`

		decoder := json.NewDecoder(bytes.NewBufferString(testData))

		report, err := evalRule(provider, decoder, EvalRuleParams{})
		if err != nil {
			t.Fatalf("error evaluating rule: %s", err)
		}

		if report.Succeeded {
			t.Fatalf("expected rule to fail")
		}
	})

}

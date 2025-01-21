// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compliance implements a specific part of the datadog-agent
// responsible for scanning host and containers and report various
// misconfigurations and compliance issues.
package compliance

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/rules/filtermodel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules/filter"
)

// seclRuleFilter defines a SECL rule filter
type seclRuleFilter struct {
	inner *filter.SECLRuleFilter
}

// newSECLRuleFilter returns a new agent version based rule filter
func newSECLRuleFilter() (*seclRuleFilter, error) {
	cfg := filtermodel.RuleFilterEventConfig{}
	model, err := filtermodel.NewRuleFilterModel(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create default SECL rule filter: %w", err)
	}

	return &seclRuleFilter{
		inner: filter.NewSECLRuleFilter(model),
	}, nil
}

// isRuleAccepted checks whether the rule is accepted
func (r *seclRuleFilter) isRuleAccepted(filters []string) (bool, error) {
	return r.inner.IsAccepted(filters)
}

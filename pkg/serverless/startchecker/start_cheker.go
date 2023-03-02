// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startchecker

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// StartingRule is the interface to define a rule
type StartingRule interface {
	ok() bool
	getError() string
}

// StartChecker is the managing the starting rules
type StartChecker struct {
	rules []StartingRule
}

// InitStartChecker creates a new StartChecker
func InitStartChecker() StartChecker {
	return StartChecker{
		rules: make([]StartingRule, 0),
	}
}

// Check loops over each rules
func (r *StartChecker) Check() bool {
	if r.rules == nil {
		log.Error("could not check as the StartChecker has not been initialized")
	}
	for _, rule := range r.rules {
		if !rule.ok() {
			log.Error(rule.getError())
			return false
		}
	}
	return true
}

// AddRule add a new rule to the StartChecker
func (r *StartChecker) AddRule(rule StartingRule) {
	if r.rules != nil {
		r.rules = append(r.rules, rule)
	} else {
		log.Error("could not add the rule as the StartChecker has not been initialized")
	}
}

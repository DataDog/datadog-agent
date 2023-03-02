// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startchecker

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type StartRule interface {
	ok() bool
	getError() string
}

type StartChecker struct {
	rules []StartRule
}

func InitStartChecker() StartChecker {
	return StartChecker{
		rules: make([]StartRule, 0),
	}
}

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

func (r *StartChecker) AddRule(rule StartRule) {
	if r.rules != nil {
		r.rules = append(r.rules, rule)
	} else {
		log.Error("could not add the rule as the StartChecker has not been initalized")
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package validators holds validators related files
package validators

import (
	"regexp"
)

// RuleIDPattern represents the regex that `RuleID`s must match
var RuleIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_]*$`)

// CheckRuleID validates a ruleID
func CheckRuleID(ruleID string) bool {
	return RuleIDPattern.MatchString(ruleID)
}

//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// SelfTestEvent is used to report a self test result
// easyjson:json
type SelfTestEvent struct {
	CustomEventCommonFields
	Success []string `json:"succeeded_tests"`
	Fails   []string `json:"failed_tests"`
}

// NewSelfTestEvent returns the rule and the result of the self test
func NewSelfTestEvent(success []string, fails []string) (*rules.Rule, *CustomEvent) {
	evt := SelfTestEvent{
		Success: success,
		Fails:   fails,
	}
	evt.FillCustomEventCommonFields()

	return NewCustomRule(SelfTestRuleID, SelfTestRuleDesc),
		NewCustomEvent(model.CustomSelfTestEventType, evt)
}

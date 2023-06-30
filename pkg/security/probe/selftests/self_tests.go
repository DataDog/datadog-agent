//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers -build_tags linux $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package selftests

import (
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
)

// SelfTestEvent is used to report a self test result
// easyjson:json
type SelfTestEvent struct {
	events.CustomEventCommonFields
	Success    []string                                `json:"succeeded_tests"`
	Fails      []string                                `json:"failed_tests"`
	TestEvents map[string]*serializers.EventSerializer `json:"test_events"`
}

// NewSelfTestEvent returns the rule and the result of the self test
func NewSelfTestEvent(success []string, fails []string, testEvents map[string]*serializers.EventSerializer) (*rules.Rule, *events.CustomEvent) {
	evt := SelfTestEvent{
		Success:    success,
		Fails:      fails,
		TestEvents: testEvents,
	}
	evt.FillCustomEventCommonFields()

	return events.NewCustomRule(events.SelfTestRuleID, events.SelfTestRuleDesc),
		events.NewCustomEvent(model.CustomSelfTestEventType, evt)
}

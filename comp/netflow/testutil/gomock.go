// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package testutil

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/golang/mock/gomock"
)

// MatchEVPFlow is a gomock matcher for Flow events sent to the EP Forwarder. It allows for
// custom assertions to be added if desired, as well as looser constraints on certain fields (for example we
// explicitly copy over flush_timestamp on events to ease exactness of tests)
func MatchEVPFlow(flowEvent *message.Message, options ...FlowMatcherOption) gomock.Matcher {
	matcher := &flowMatcher{
		expected: flowEvent,
	}

	for _, option := range options {
		option(matcher)
	}

	return matcher
}

// flowMatcher implements the gomock interface, users should access it using MatchEVPFlow
type flowMatcher struct {
	expected *message.Message

	customAssertions []func(expected, actual map[string]any) bool

	// Some tests historically check for an exact flush_date. This bool allows that behavior to continue if desired.
	RequireExactMatch bool
}

// FlowMatcherOption is a function that can be passed to MatchEVPFlow to modify the matcher behavior in certain ways
type FlowMatcherOption func(*flowMatcher)

// WithAssertion allows for custom assertions to be added to the matcher, with access to the raw json event data
func WithAssertion(assertion func(expected, actual map[string]any) bool) FlowMatcherOption {
	return func(matcher *flowMatcher) {
		matcher.customAssertions = append(matcher.customAssertions, assertion)
	}
}

// Matches implements the gomock interface. It performs some deserialization to allow for remapping the flush_timestamp
func (f *flowMatcher) Matches(x interface{}) bool {
	if f.RequireExactMatch {
		// legacy behavior requires timestamps to be exact matches, so we can just compare the full event body
		return gomock.Eq(f.expected).Matches(x)
	}

	// parse the message incoming message + original
	actualMessage, ok := x.(*message.Message)
	if !ok {
		return false
	}

	actualEvent := make(map[string]any)
	if err := json.Unmarshal(actualMessage.GetContent(), &actualEvent); err != nil {
		return false
	}

	expectedEvent := make(map[string]any)
	if err := json.Unmarshal(f.expected.GetContent(), &expectedEvent); err != nil {
		return false
	}

	// update the timestamp
	expectedEvent["flush_timestamp"] = actualEvent["flush_timestamp"]

	// reserialize both expected + actual - json.Marshal can lead to different serialization order
	// after parsing into a map, so make sure both are reserialized before comparing
	expectedJSONAfterUpdate, err := json.Marshal(expectedEvent)
	if err != nil {
		return false
	}
	actualJSONAfterUpdate, err := json.Marshal(actualEvent)
	if err != nil {
		return false
	}

	expected := message.NewMessage(expectedJSONAfterUpdate, f.expected.Origin, f.expected.Status, f.expected.IngestionTimestamp)
	actual := message.NewMessage(actualJSONAfterUpdate, actualMessage.Origin, actualMessage.Status, actualMessage.IngestionTimestamp)

	doesGoMockMatch := gomock.Eq(expected).Matches(actual)
	doCustomAssertionsMatch := true
	for _, assertion := range f.customAssertions {
		doCustomAssertionsMatch = doCustomAssertionsMatch && assertion(expectedEvent, actualEvent)
	}

	return doesGoMockMatch && doCustomAssertionsMatch
}

// String implements the gomock interface according to the gomock.Eq() implementation
func (f *flowMatcher) String() string {
	return fmt.Sprintf("is equal to %v (%T) (requireExactMatch=%v)", f.expected, f.expected, f.RequireExactMatch)
}

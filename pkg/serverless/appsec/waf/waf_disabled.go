// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Build when CGO is disabled or the target OS or Arch are not supported
//go:build !cgo || windows || !(amd64 || arm64)

package waf

import (
	"errors"
	"time"
)

type (
	// Handle represents an instance of the WAF for a given ruleset.
	Handle struct{}
	// Context is a WAF execution context.
	Context struct{}
)

var errDisabledReason = errors.New(disabledReason)

// Health allows knowing if the WAF can be used. It returns a nil error when the WAF library is healthy.
// Otherwise, it returns an error describing the issue.
func Health() error { return errDisabledReason }

// Version returns the current version of the WAF
func Version() string { return "" }

// NewHandle creates a new instance of the WAF with the given JSON rule.
func NewHandle([]byte, string, string) (*Handle, error) { return nil, errDisabledReason }

// Addresses returns the list of addresses the WAF rule is expecting.
func (*Handle) Addresses() []string { return nil }

// RulesetInfo returns the rules initialization metrics for the current WAF handle
func (*Handle) RulesetInfo() RulesetInfo { return RulesetInfo{} }

// UpdateRuleData updates the data that some rules reference to.
// The given rule data must be a raw JSON string of the form
// [ {rule data #1}, ... {rule data #2} ]
func (*Handle) UpdateRuleData([]byte) error { return errDisabledReason }

// Close the WAF and release the underlying C memory as soon as there are
// no more WAF contexts using the rule.
func (*Handle) Close() {}

// NewContext a new WAF context and increase the number of references to the WAF
// handle. A nil value is returned when the WAF handle can no longer be used
// or the WAF context couldn't be created.
func NewContext(*Handle) *Context { return nil }

// Run the WAF with the given Go values and timeout.
func (*Context) Run(map[string]interface{}, time.Duration) ([]byte, []string, error) {
	return nil, nil, errDisabledReason
}

// TotalRuntime returns the cumulated WAF runtime across various run calls within the same WAF context.
// Returned time is in nanoseconds.
func (*Context) TotalRuntime() (uint64, uint64) { return 0, 0 }

// TotalTimeouts returns the cumulated amount of WAF timeouts across various run calls within the same WAF context.
func (*Context) TotalTimeouts() uint64 { return 0 }

// Close the WAF context by releasing its C memory and decreasing the number of
// references to the WAF handle.
func (*Context) Close() {}

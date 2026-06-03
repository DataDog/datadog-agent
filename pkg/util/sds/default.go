// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sds wraps the Datadog Sensitive Data Scanner shared library and
// exposes a small scanner API (creation, reconfiguration and scanning). It is
// only fully functional when the Agent is compiled with the `sds` build tag and
// the shared library is available, otherwise a no-op mock is used.
package sds

import (
	"encoding/json"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DefaultEmailRuleID is the id of the built-in email address rule the default
// scanner is configured with.
const DefaultEmailRuleID = "PuXiVTCkTHOtj0Yad1ppsw"

var (
	defaultScanner     *Scanner
	defaultScannerOnce sync.Once
)

// defaultRulesConfig returns the raw rules configuration the default scanner is
// bootstrapped with: a single basic email address scanner that redacts matches.
func defaultRulesConfig() []byte {
	config := RulesConfig{
		IsEnabled: true,
		Rules: []RuleConfig{
			{
				ID:   DefaultEmailRuleID,
				Name: "Standard Email Address Scanner",
				Definition: RuleDefinition{
					// basic alphanumeric@alphanumeric.alphanumeric pattern.
					Pattern: `[a-zA-Z0-9]+@[a-zA-Z0-9]+\.[a-zA-Z0-9]+`,
				},
				MatchAction: MatchAction{
					Type:        "Redact",
					Placeholder: "[redacted]",
				},
				IsEnabled: true,
			},
		},
	}
	raw, _ := json.Marshal(config)
	return raw
}

// DefaultScanner returns the process-wide default SDS scanner, lazily creating
// it on first use and configuring it with the built-in rules (see
// defaultRulesConfig).
func DefaultScanner() *Scanner {
	defaultScannerOnce.Do(func() {
		defaultScanner = CreateScanner()
		if err := defaultScanner.Reconfigure(ReconfigureOrder{Type: DatadogRules, Config: defaultRulesConfig()}); err != nil {
			log.Errorf("Can't configure the default SDS scanner: %v", err)
		}
	})
	return defaultScanner
}

// Reconfigure reconfigures the default scanner with the given order.
func Reconfigure(order ReconfigureOrder) error {
	return DefaultScanner().Reconfigure(order)
}

// Scan scans the given event through the default scanner. It returns whether
// the event was mutated, the processed event (nil when not mutated) and an
// error if the scanner is not ready.
func Scan(event []byte) (bool, []byte, error) {
	return DefaultScanner().Scan(event)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !sds

//nolint:revive
package sds

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const SDSEnabled = false

// Scanner mock.
type Scanner struct {
}

// Match mock.
type Match struct {
	RuleIdx uint32
}

// CreateScanner creates a scanner for unsupported platforms/architectures.
func CreateScanner(_ int) *Scanner {
	return nil
}

// Reconfigure mocks the Reconfigure function.
func (s *Scanner) Reconfigure(_ ReconfigureOrder) error {
	return nil
}

// Delete mocks the Delete function.
func (s *Scanner) Delete() {}

// GetRuleByIdx mocks the GetRuleByIdx function.
func (s *Scanner) GetRuleByIdx(_ uint32) (RuleConfig, error) {
	return RuleConfig{}, nil
}

// IsReady mocks the IsReady function.
func (s *Scanner) IsReady() bool { return false }

// IsConfigured mocks the IsConfigured function.
func (s *Scanner) IsConfigured() bool { return true }

// Scan mocks the Scan function.
func (s *Scanner) Scan(_ []byte, _ *message.Message) (bool, []byte, error) {
	return false, nil, nil
}

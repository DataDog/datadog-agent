// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
)

const (
	// DefaultTimeout holds the default duration for checks timeout
	DefaultTimeout = 30 * time.Second
)

// Check is the interface for compliance checks
type Check check.Check

// CheckStatus describes current status for a check
type CheckStatus struct {
	RuleID      string
	Name        string
	Description string
	Version     string
	Framework   string
	Source      string
	InitError   error
	LastEvent   *event.Event
}

// CheckStatusList describes status for all configured checks
type CheckStatusList []*CheckStatus

// CheckVisitor defines a visitor func for compliance checks
type CheckVisitor func(rule *RuleCommon, check Check, err error) bool

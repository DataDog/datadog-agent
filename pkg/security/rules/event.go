// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package rules

import "github.com/DataDog/datadog-agent/pkg/security/secl/eval"

// RuleEvent - Rule event wrapper used to send an event to the backend
type RuleEvent struct {
	RuleID string     `json:"rule_id"`
	Event  eval.Event `json:"event"`
}

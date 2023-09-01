// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

package selftests

import (
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// SelfTester represents all the state needed to conduct rule injection test at startup
type SelfTester struct{}

// IsExpectedEvent sends an event to the tester
func (t *SelfTester) IsExpectedEvent(rule *rules.Rule, event eval.Event, p *probe.Probe) bool {
	return false
}

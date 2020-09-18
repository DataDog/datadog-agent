// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

// NewRuleSet returns a new rule set
func (p *Probe) NewRuleSet(opts *rules.Opts) *rules.RuleSet {
	eventCtor := func() eval.Event {
		return NewEvent(p.resolvers)
	}

	return rules.NewRuleSet(&Model{}, eventCtor, opts)
}

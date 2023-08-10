// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !functionaltests

package rules

import "github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"

type EventCollector struct {
}

func (ec *EventCollector) CollectEvent(rs *RuleSet, event eval.Event, result bool) {
	// no-op
}

func (ec *EventCollector) Stop() []CollectedEvent {
	return nil
}

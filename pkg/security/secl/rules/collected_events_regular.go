// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !functionaltests

// Package rules holds rules related files
package rules

import "github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"

// EventCollector defines an event collector
type EventCollector struct {
}

// CollectEvent collects event
//
//nolint:revive // TODO(SEC) Fix revive linter
func (ec *EventCollector) CollectEvent(rs *RuleSet, event eval.Event, result bool) {
	// no-op
}

// Stop stops the event collector
func (ec *EventCollector) Stop() []CollectedEvent {
	return nil
}

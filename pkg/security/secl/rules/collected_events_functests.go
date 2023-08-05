// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

package rules

import "github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"

type CollectedEvent struct {
	Type       string
	EvalResult bool
	Fields     map[string]interface{}
}

type EventCollector struct {
	eventsCollected []CollectedEvent
}

func (ec *EventCollector) CollectEvent(rs *RuleSet, event eval.Event, result bool) {
	collectedEvent := CollectedEvent{
		Type:       event.GetType(),
		EvalResult: result,
		Fields:     make(map[string]interface{}, len(rs.fields)),
	}

	for _, field := range rs.fields {
		value, err := event.GetFieldValue(field)
		if err != nil {
			rs.logger.Errorf("failed to get value for %s: %v", field, err)
			continue
		}

		collectedEvent.Fields[field] = value
	}

	ec.eventsCollected = append(ec.eventsCollected, collectedEvent)
}

func (ec *EventCollector) Stop() []CollectedEvent {
	collected := ec.eventsCollected
	ec.eventsCollected = nil
	return collected
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package rules holds rules related files
package rules

import (
	"errors"
	"slices"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

type EventCollector struct {
	sync.Mutex
	eventsCollected []CollectedEvent
}

func (ec *EventCollector) CollectEvent(rs *RuleSet, ctx *eval.Context, event eval.Event, result bool) {
	ec.Lock()
	defer ec.Unlock()
	var fieldNotSupportedError *eval.ErrNotSupported

	eventType := event.GetType()
	collectedEvent := CollectedEvent{
		Type:       eventType,
		EvalResult: result,
		Fields:     make(map[string]interface{}, len(rs.fields)),
	}

	resolvedFields := ctx.GetResolvedFields()

	for _, field := range rs.fields {
		// skip fields that have not been resolved
		if !slices.Contains(resolvedFields, field) {
			continue
		}

		fieldEventType, err := event.GetFieldEventType(field)
		if err != nil {
			rs.logger.Errorf("failed to get event type for field %s: %v", field, err)
		}

		if fieldEventType != "" && fieldEventType != eventType {
			continue
		}

		value, err := event.GetFieldValue(field)
		if err != nil {
			// GetFieldValue returns the default type value with ErrNotSupported in case the field Check test fails
			if !errors.As(err, &fieldNotSupportedError) {
				rs.logger.Errorf("failed to get value for %s: %v", field, err)
				continue
			}
		}

		collectedEvent.Fields[field] = value
	}

	ec.eventsCollected = append(ec.eventsCollected, collectedEvent)
}

func (ec *EventCollector) Stop() []CollectedEvent {
	ec.Lock()
	defer ec.Unlock()

	collected := ec.eventsCollected
	ec.eventsCollected = nil
	return collected
}

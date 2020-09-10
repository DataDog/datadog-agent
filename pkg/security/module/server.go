// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package module

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-go/statsd"
	"math"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EventServer represents a gRPC server in charge of receiving events sent by
// the runtime security system-probe module and forwards them to Datadog
type EventServer struct {
	msgs          chan *api.SecurityEventMessage
	droppedEvents map[string]*int64
}

// GetEvents waits for security events
func (e *EventServer) GetEvents(params *api.GetParams, stream api.SecurityModule_GetEventsServer) error {
	msgs := 10
LOOP:
	for {
		select {
		case msg := <-e.msgs:
			if err := stream.Send(msg); err != nil {
				return err
			}
			msgs--
		case <-time.After(time.Second):
			break LOOP
		}

		if msgs <= 0 {
			break
		}
	}

	return nil
}

// SendEvent forwards events sent by the runtime security module to Datadog
func (e *EventServer) SendEvent(rule *eval.Rule, event eval.Event) {
	data, err := json.Marshal(rules.RuleEvent{Event: event, RuleID: rule.ID})
	if err != nil {
		return
	}
	tags := append(rule.Tags, "rule_id:"+rule.ID)
	tags = append(tags, event.(*sprobe.Event).GetTags()...)
	log.Infof("Sending event message for rule `%s` to security-agent `%s` with tags %v", rule.ID, string(data), tags)

	msg := &api.SecurityEventMessage{
		RuleID: rule.ID,
		Type:   event.GetType(),
		Tags:   tags,
		Data:   data,
	}

	select {
	case e.msgs <- msg:
		break
	default:
		// Update stats
		eventsStat, ok := e.droppedEvents[rule.ID]
		if ok {
			atomic.AddInt64(eventsStat, 1)
		}
		// Do not wait for the channel to free up, we don't want to delay the processing pipeline further
		log.Warnf("the event server channel is full, an event of ID %v was dropped", msg.RuleID)
		break
	}
}

// GetStats returns a map indexed by ruleIDs that describes the amount of events
// that were dropped because the groc channel was full
func (e *EventServer) GetStats() map[string]int64 {
	stats := make(map[string]int64)
	for ruleID, val := range e.droppedEvents {
		stats[ruleID] = atomic.SwapInt64(val, 0)
	}
	return stats
}

// SendStats sends statistics about the number of dropped events
func (e *EventServer) SendStats(client *statsd.Client) error {
	for ruleID, val := range e.GetStats() {
		tags := []string{fmt.Sprintf("rule_id:%s", ruleID)}
		if val > 0 {
			if err := client.Count(sprobe.MetricPrefix+".rules.event_server.drop", val, tags, 1.0); err != nil {
				return err
			}
		}
	}
	return nil
}

// NewEventServer returns a new gRPC event server
func NewEventServer(ids []string) *EventServer {
	es := &EventServer{
		msgs:          make(chan *api.SecurityEventMessage, defaultBurst*int(math.Min(float64(len(ids)), 50))),
		droppedEvents: make(map[string]*int64),
	}
	for _, id := range ids {
		var val int64
		es.droppedEvents[id] = &val
	}
	return es
}

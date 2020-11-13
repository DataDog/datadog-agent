// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package module

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EventServer represents a gRPC server in charge of receiving events sent by
// the runtime security system-probe module and forwards them to Datadog
type EventServer struct {
	msgs          chan *api.SecurityEventMessage
	expiredEvents map[string]*int64
	rate          *Limiter
}

// GetEvents waits for security events
func (e *EventServer) GetEvents(params *api.GetParams, stream api.SecurityModule_GetEventsServer) error {
	// Read 10 security events per call
	msgs := 10
LOOP:
	for {
		// Check that the limit is not reached
		if !e.rate.limiter.Allow() {
			return nil
		}

		// Read on message
		select {
		case msg := <-e.msgs:
			if err := stream.Send(msg); err != nil {
				return err
			}
			msgs--
		case <-time.After(time.Second):
			break LOOP
		}

		// Stop the loop when 10 messages were retrieved
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
	log.Tracef("Sending event message for rule `%s` to security-agent `%s` with tags %v", rule.ID, string(data), tags)

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
		// The channel is full, consume the oldest event
		oldestMsg := <-e.msgs
		// Try to send the event again
		select {
		case e.msgs <- msg:
			break
		default:
			// Looks like the channel is full again, expire the current message too
			e.expireEvent(msg)
			break
		}
		e.expireEvent(oldestMsg)
		break
	}
}

// expireEvent updates the count of expired messages for the appropriate rule
func (e *EventServer) expireEvent(msg *api.SecurityEventMessage) {
	// Update metric
	count, ok := e.expiredEvents[msg.RuleID]
	if ok {
		atomic.AddInt64(count, 1)
	}
	log.Tracef("the event server channel is full, an event of ID %v was dropped", msg.RuleID)
}

// GetStats returns a map indexed by ruleIDs that describes the amount of events
// that were expired or rate limited before reaching
func (e *EventServer) GetStats() map[string]int64 {
	stats := make(map[string]int64)
	for ruleID, val := range e.expiredEvents {
		stats[ruleID] = atomic.SwapInt64(val, 0)
	}
	return stats
}

// SendStats sends statistics about the number of dropped events
func (e *EventServer) SendStats(client *statsd.Client) error {
	for ruleID, val := range e.GetStats() {
		tags := []string{fmt.Sprintf("rule_id:%s", ruleID)}
		if val > 0 {
			if err := client.Count(sprobe.MetricPrefix+".rules.event_server.expired", val, tags, 1.0); err != nil {
				return err
			}
		}
	}
	return nil
}

// NewEventServer returns a new gRPC event server
func NewEventServer(ids []string, cfg *config.Config) *EventServer {
	es := &EventServer{
		msgs:          make(chan *api.SecurityEventMessage, cfg.EventServerBurst*3),
		expiredEvents: make(map[string]*int64),
		rate:          NewLimiter(rate.Limit(cfg.EventServerRate), cfg.EventServerBurst),
	}
	for _, id := range ids {
		var val int64
		es.expiredEvents[id] = &val
	}
	return es
}

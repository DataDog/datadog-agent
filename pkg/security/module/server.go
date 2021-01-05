// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package module

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/pkg/errors"
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
	sync.RWMutex
	msgs          chan *api.SecurityEventMessage
	expiredEvents map[rules.RuleID]*int64
	rate          *Limiter
	statsdClient  *statsd.Client
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
func (e *EventServer) SendEvent(rule *rules.Rule, event eval.Event) {
	agentContext := &AgentContext{
		RuleID: rule.Definition.ID,
		Tags:   append(rule.Tags, "rule_id:"+rule.Definition.ID),
	}

	ruleEvent := &Signal{
		Title:        rule.Definition.ID,
		Msg:          rule.Definition.Description,
		AgentContext: agentContext,
	}

	if policy := rule.Definition.Policy; policy != nil {
		agentContext.PolicyName = policy.Name
		agentContext.PolicyVersion = policy.Version
	}

	probeJSON, err := json.Marshal(event)
	if err != nil {
		log.Error(errors.Wrap(err, "failed to marshal event"))
		return
	}

	ruleEventJSON, err := json.Marshal(ruleEvent)
	if err != nil {
		log.Error(errors.Wrap(err, "failed to marshal event context"))
		return
	}

	data := append(probeJSON[:len(probeJSON)-1], ',')
	data = append(data, ruleEventJSON[1:]...)
	log.Tracef("Sending event message for rule `%s` to security-agent `%s`", rule.ID, string(data))

	msg := &api.SecurityEventMessage{
		RuleID: rule.Definition.ID,
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
	e.RLock()
	defer e.RUnlock()

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
	e.RLock()
	defer e.RUnlock()

	stats := make(map[string]int64)
	for ruleID, val := range e.expiredEvents {
		stats[ruleID] = atomic.SwapInt64(val, 0)
	}
	return stats
}

// SendStats sends statistics about the number of dropped events
func (e *EventServer) SendStats() error {
	for ruleID, val := range e.GetStats() {
		tags := []string{fmt.Sprintf("rule_id:%s", ruleID)}
		if val > 0 {
			if err := e.statsdClient.Count(sprobe.MetricPrefix+".rules.event_server.expired", val, tags, 1.0); err != nil {
				return err
			}
		}
	}
	return nil
}

// Apply a rule set
func (e *EventServer) Apply(ruleIDs []rules.RuleID) {
	e.Lock()
	defer e.Unlock()

	e.expiredEvents = make(map[rules.RuleID]*int64)
	for _, id := range ruleIDs {
		e.expiredEvents[id] = new(int64)
	}
}

// NewEventServer returns a new gRPC event server
func NewEventServer(cfg *config.Config, client *statsd.Client) *EventServer {
	es := &EventServer{
		msgs:          make(chan *api.SecurityEventMessage, cfg.EventServerBurst*3),
		expiredEvents: make(map[rules.RuleID]*int64),
		rate:          NewLimiter(rate.Limit(cfg.EventServerRate), cfg.EventServerBurst),
		statsdClient:  client,
	}
	return es
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows
// +build linux windows

package module

import (
	"context"
	json "encoding/json"
	"fmt"
	"strings"
	"time"

	easyjson "github.com/mailru/easyjson"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type pendingMsg struct {
	ruleID    string
	data      []byte
	tags      map[string]bool
	service   string
	extTagsCb func() []string
	sendAfter time.Time
}

// GetActivityDumpStream waits for activity dumps and forwards them to the stream
func (a *APIServer) GetActivityDumpStream(params *api.ActivityDumpStreamParams, stream api.SecurityModule_GetActivityDumpStreamServer) error {
	// read one activity dump or timeout after one second
	select {
	case dump := <-a.activityDumps:
		if err := stream.Send(dump); err != nil {
			return err
		}
	case <-time.After(time.Second):
		break
	}
	return nil
}

// SendActivityDump queues an activity dump to the chan of activity dumps
func (a *APIServer) SendActivityDump(dump *api.ActivityDumpStreamMessage) {
	// send the dump to the channel
	select {
	case a.activityDumps <- dump:
		break
	default:
		// The channel is full, consume the oldest dump
		oldestDump := <-a.activityDumps
		// Try to send the event again
		select {
		case a.activityDumps <- dump:
			break
		default:
			// Looks like the channel is full again, expire the current message too
			a.expireDump(dump)
			break
		}
		a.expireDump(oldestDump)
		break
	}
}

// GetProcessEvents sends process events through a gRPC stream
func (a *APIServer) GetProcessEvents(params *api.GetProcessEventParams, stream api.SecurityModule_GetProcessEventsServer) error {
	// Read 10 security events per call
	msgs := 10
LOOP:
	for {
		// Read on message
		select {
		case msg := <-a.processMsgs:
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

// RuleEvent is a wrapper used to send an event to the backend
type RuleEvent struct {
	RuleID string `json:"rule_id"`
	Event  Event  `json:"event"`
}

func (a *APIServer) enqueue(msg *pendingMsg) {
	a.queueLock.Lock()
	a.queue = append(a.queue, msg)
	a.queueLock.Unlock()
}

func (a *APIServer) dequeue(now time.Time, cb func(msg *pendingMsg)) {
	a.queueLock.Lock()
	defer a.queueLock.Unlock()

	var i int
	var msg *pendingMsg

	for i != len(a.queue) {
		msg = a.queue[i]
		if msg.sendAfter.After(now) {
			break
		}
		cb(msg)

		i++
	}

	if i >= len(a.queue) {
		a.queue = a.queue[0:0]
	} else if i > 0 {
		a.queue = a.queue[i:]
	}
}

func (a *APIServer) start(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case now := <-ticker.C:
			a.dequeue(now, func(msg *pendingMsg) {
				for _, tag := range msg.extTagsCb() {
					msg.tags[tag] = true
				}

				// recopy tags
				var tags []string
				hasService := len(msg.service) != 0
				for tag := range msg.tags {
					tags = append(tags, tag)

					// look for the service tag if we don't have one yet
					if !hasService {
						if strings.HasPrefix(tag, "service:") {
							msg.service = strings.TrimPrefix(tag, "service:")
							hasService = true
						}
					}
				}

				m := &api.SecurityEventMessage{
					RuleID:  msg.ruleID,
					Data:    msg.data,
					Service: msg.service,
					Tags:    tags,
				}

				select {
				case a.msgs <- m:
					break
				default:
					// The channel is full, consume the oldest event
					oldestMsg := <-a.msgs
					// Try to send the event again
					select {
					case a.msgs <- m:
						break
					default:
						// Looks like the channel is full again, expire the current message too
						a.expireEvent(m)
						break
					}
					a.expireEvent(oldestMsg)
					break
				}
			})
		case <-ctx.Done():
			return
		}
	}
}

// Start the api server, starts to consume the msg queue
func (a *APIServer) Start(ctx context.Context) {
	go a.start(ctx)
}

// GetConfig returns config of the runtime security module required by the security agent
func (a *APIServer) GetConfig(ctx context.Context, params *api.GetConfigParams) (*api.SecurityConfigMessage, error) {
	if a.cfg != nil {
		return &api.SecurityConfigMessage{
			FIMEnabled:     a.cfg.FIMEnabled,
			RuntimeEnabled: a.cfg.RuntimeEnabled,
		}, nil
	}
	return &api.SecurityConfigMessage{}, nil
}

// SendEvent forwards events sent by the runtime security module to Datadog
func (a *APIServer) SendEvent(rule *rules.Rule, event Event, extTagsCb func() []string, service string) {
	agentContext := AgentContext{
		RuleID:      rule.Definition.ID,
		RuleVersion: rule.Definition.Version,
		Version:     version.AgentVersion,
	}

	ruleEvent := &Signal{
		Title:        rule.Definition.Description,
		AgentContext: agentContext,
	}

	if policy := rule.Definition.Policy; policy != nil {
		ruleEvent.AgentContext.PolicyName = policy.Name
		ruleEvent.AgentContext.PolicyVersion = policy.Version
	}

	probeJSON, err := json.Marshal(event)
	if err != nil {
		seclog.Errorf("failed to marshal event: %v", err)
		return
	}

	ruleEventJSON, err := easyjson.Marshal(ruleEvent)
	if err != nil {
		seclog.Errorf("failed to marshal event context: %v", err)
		return
	}

	data := append(probeJSON[:len(probeJSON)-1], ',')
	data = append(data, ruleEventJSON[1:]...)
	seclog.Tracef("Sending event message for rule `%s` to security-agent `%s`", rule.ID, string(data))

	msg := &pendingMsg{
		ruleID:    rule.Definition.ID,
		data:      data,
		extTagsCb: extTagsCb,
		tags:      make(map[string]bool),
		service:   service,
		sendAfter: time.Now().Add(a.retention),
	}

	msg.tags["rule_id:"+rule.Definition.ID] = true

	for _, tag := range rule.Tags {
		msg.tags[tag] = true
	}

	for _, tag := range event.GetTags() {
		msg.tags[tag] = true
	}

	a.enqueue(msg)
}

// expireEvent updates the count of expired messages for the appropriate rule
func (a *APIServer) expireEvent(msg *api.SecurityEventMessage) {
	a.expiredEventsLock.RLock()
	defer a.expiredEventsLock.RUnlock()

	// Update metric
	count, ok := a.expiredEvents[msg.RuleID]
	if ok {
		count.Inc()
	}
	seclog.Tracef("the event server channel is full, an event of ID %v was dropped", msg.RuleID)
}

// expireDump updates the count of expired dumps
func (a *APIServer) expireDump(dump *api.ActivityDumpStreamMessage) {
	a.expiredDumpsLock.Lock()
	defer a.expiredDumpsLock.Unlock()

	// update metric
	_ = a.expiredDumps.Inc()
	seclog.Tracef("the activity dump server channel is full, a dump of [%s] was dropped\n", dump.GetDump().GetMetadata().GetName())
}

// GetStats returns a map indexed by ruleIDs that describes the amount of events
// that were expired or rate limited before reaching
func (a *APIServer) GetStats() map[string]int64 {
	a.expiredEventsLock.RLock()
	defer a.expiredEventsLock.RUnlock()

	stats := make(map[string]int64)
	for ruleID, val := range a.expiredEvents {
		stats[ruleID] = val.Swap(0)
	}
	return stats
}

// SendStats sends statistics about the number of dropped events
func (a *APIServer) SendStats() error {
	for ruleID, val := range a.GetStats() {
		tags := []string{fmt.Sprintf("rule_id:%s", ruleID)}
		if val > 0 {
			if err := a.statsdClient.Count(metrics.MetricEventServerExpired, val, tags, 1.0); err != nil {
				return err
			}
		}
	}

	if count := a.expiredProcessEvents.Swap(0); count > 0 {
		if err := a.statsdClient.Count(metrics.MetricProcessEventsServerExpired, count, []string{}, 1.0); err != nil {
			return err
		}
	}
	return nil
}

// ReloadPolicies reloads the policies
func (a *APIServer) ReloadPolicies(ctx context.Context, params *api.ReloadPoliciesParams) (*api.ReloadPoliciesResultMessage, error) {
	if err := a.module.ReloadPolicies(); err != nil {
		return nil, err
	}
	return &api.ReloadPoliciesResultMessage{}, nil
}

// Apply a rule set
func (a *APIServer) Apply(ruleIDs []rules.RuleID) {
	a.expiredEventsLock.Lock()
	defer a.expiredEventsLock.Unlock()

	a.expiredEvents = make(map[rules.RuleID]*atomic.Int64)
	for _, id := range ruleIDs {
		a.expiredEvents[id] = atomic.NewInt64(0)
	}
}

// SendProcessEvent forwards collected process events to the processMsgs channel so they can be consumed next time GetProcessEvents
// is called
func (a *APIServer) SendProcessEvent(data []byte) {
	m := &api.SecurityProcessEventMessage{
		Data: data,
	}

	select {
	case a.processMsgs <- m:
		break
	default:
		// The channel is full, expire the oldest event
		<-a.processMsgs
		a.expiredProcessEvents.Inc()
		// Try to send the event again
		select {
		case a.processMsgs <- m:
			break
		default:
			// looks like the process msgs channel is full again, expire the current event
			a.expiredProcessEvents.Inc()
			break
		}
		break
	}
}

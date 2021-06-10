// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package module

import (
	"context"
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
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type pendingMsg struct {
	ruleID    string
	data      []byte
	tags      map[string]bool
	extTagsCb func() []string
	sendAfter time.Time
}

// APIServer represents a gRPC server in charge of receiving events sent by
// the runtime security system-probe module and forwards them to Datadog
type APIServer struct {
	sync.RWMutex
	msgs              chan *api.SecurityEventMessage
	expiredEventsLock sync.RWMutex
	expiredEvents     map[rules.RuleID]*int64
	rate              *Limiter
	statsdClient      *statsd.Client
	probe             *sprobe.Probe
	queue             []*pendingMsg
	retention         time.Duration
	cfg               *config.Config
}

// GetEvents waits for security events
func (a *APIServer) GetEvents(params *api.GetEventParams, stream api.SecurityModule_GetEventsServer) error {
	// Read 10 security events per call
	msgs := 10
LOOP:
	for {
		// Check that the limit is not reached
		if !a.rate.limiter.Allow() {
			return nil
		}

		// Read on message
		select {
		case msg := <-a.msgs:
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

// Event is the interface that an event must implement to be sent to the backend
type Event interface {
	GetTags() []string
	GetType() string
}

// RuleEvent is a wrapper used to send an event to the backend
type RuleEvent struct {
	RuleID string `json:"rule_id"`
	Event  Event  `json:"event"`
}

// DumpProcessCache handle process dump cache requests
func (a *APIServer) DumpProcessCache(ctx context.Context, params *api.DumpProcessCacheParams) (*api.SecurityDumpProcessCacheMessage, error) {
	resolvers := a.probe.GetResolvers()

	filename, err := resolvers.ProcessResolver.Dump()
	if err != nil {
		return nil, err
	}

	return &api.SecurityDumpProcessCacheMessage{
		Filename: filename,
	}, nil
}

func (a *APIServer) enqueue(msg *pendingMsg) {
	a.Lock()
	a.queue = append(a.queue, msg)
	a.Unlock()
}

func (a *APIServer) dequeue(now time.Time, cb func(msg *pendingMsg)) {
	a.Lock()
	defer a.Unlock()

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
				for tag := range msg.tags {
					tags = append(tags, tag)
				}

				m := &api.SecurityEventMessage{
					RuleID: msg.ruleID,
					Data:   msg.data,
					Tags:   tags,
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
func (a *APIServer) SendEvent(rule *rules.Rule, event Event, extTagsCb func() []string) {
	agentContext := &AgentContext{
		RuleID:      rule.Definition.ID,
		RuleVersion: rule.Definition.Version,
	}

	ruleEvent := &Signal{
		Title:        rule.Definition.Description,
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

	msg := &pendingMsg{
		ruleID:    rule.Definition.ID,
		data:      data,
		extTagsCb: extTagsCb,
		tags:      make(map[string]bool),
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
		atomic.AddInt64(count, 1)
	}
	log.Tracef("the event server channel is full, an event of ID %v was dropped", msg.RuleID)
}

// GetStats returns a map indexed by ruleIDs that describes the amount of events
// that were expired or rate limited before reaching
func (a *APIServer) GetStats() map[string]int64 {
	a.RLock()
	defer a.RUnlock()

	stats := make(map[string]int64)
	for ruleID, val := range a.expiredEvents {
		stats[ruleID] = atomic.SwapInt64(val, 0)
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
	return nil
}

// Apply a rule set
func (a *APIServer) Apply(ruleIDs []rules.RuleID) {
	a.expiredEventsLock.Lock()
	defer a.expiredEventsLock.Unlock()

	a.expiredEvents = make(map[rules.RuleID]*int64)
	for _, id := range ruleIDs {
		a.expiredEvents[id] = new(int64)
	}
}

// NewAPIServer returns a new gRPC event server
func NewAPIServer(cfg *config.Config, probe *sprobe.Probe, client *statsd.Client) *APIServer {
	es := &APIServer{
		msgs:          make(chan *api.SecurityEventMessage, cfg.EventServerBurst*3),
		expiredEvents: make(map[rules.RuleID]*int64),
		rate:          NewLimiter(rate.Limit(cfg.EventServerRate), cfg.EventServerBurst),
		statsdClient:  client,
		probe:         probe,
		retention:     time.Duration(cfg.EventServerRetention) * time.Second,
		cfg:           cfg,
	}
	return es
}

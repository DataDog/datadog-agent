// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package module holds module related files
package module

import (
	"context"
	json "encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/mailru/easyjson"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/types/known/timestamppb"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	compression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	pconfig "github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/probe/selftests"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/rules/monitor"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// events delay is used for 2 actions: hash and kills:
	// - for hash actions, the reports will be marked as resolved after MAX 5 sec (so
	//   it doesn't matter if this retry period lasts for longer)
	// - for kill actions:
	//   . a kill can be queued up to the end of the first disarmer period (1min by default)
	//   . so, we set the server retry period to 1min and 2sec (+2sec to have the
	//     time to trigger the kill and wait to catch the process exit)
	maxRetry   = 62
	retryDelay = time.Second
)

type pendingMsg struct {
	ruleID          string
	backendEvent    events.BackendEvent
	eventSerializer *serializers.EventSerializer
	tags            []string
	actionReports   []model.ActionReport
	service         string
	timestamp       time.Time
	extTagsCb       func() ([]string, bool)
	sendAfter       time.Time
	retry           int
}

func (p *pendingMsg) isResolved() bool {
	for _, report := range p.actionReports {
		if err := report.IsResolved(); err != nil {
			seclog.Debugf("action report not resolved: %v", err)
			return false
		}
	}
	return true
}

func (p *pendingMsg) toJSON() ([]byte, error) {
	p.backendEvent.RuleActions = []json.RawMessage{}

	for _, report := range p.actionReports {
		if patcher, ok := report.(serializers.EventSerializerPatcher); ok {
			patcher.PatchEvent(p.eventSerializer)
		}

		data, err := report.ToJSON()
		if err != nil {
			return nil, err
		}

		if len(data) > 0 {
			p.backendEvent.RuleActions = append(p.backendEvent.RuleActions, data)
		}
	}

	backendEventJSON, err := easyjson.Marshal(p.backendEvent)
	if err != nil {
		return nil, err
	}

	eventJSON, err := p.eventSerializer.ToJSON()
	if err != nil {
		return nil, err
	}

	return mergeJSON(backendEventJSON, eventJSON)
}

func mergeJSON(j1, j2 []byte) ([]byte, error) {
	if len(j1) == 0 || len(j2) == 0 {
		return nil, errors.New("malformed json")
	}

	data := append(j1[:len(j1)-1], ',')
	return append(data, j2[1:]...), nil
}

// APIServer represents a gRPC server in charge of receiving events sent by
// the runtime security system-probe module and forwards them to Datadog
type APIServer struct {
	api.UnimplementedSecurityModuleServer
	msgs               chan *api.SecurityEventMessage
	activityDumps      chan *api.ActivityDumpStreamMessage
	expiredEventsLock  sync.RWMutex
	expiredEvents      map[rules.RuleID]*atomic.Int64
	expiredDumps       *atomic.Int64
	statsdClient       statsd.ClientInterface
	probe              *sprobe.Probe
	queueLock          sync.Mutex
	queue              []*pendingMsg
	retention          time.Duration
	cfg                *config.RuntimeSecurityConfig
	selfTester         *selftests.SelfTester
	cwsConsumer        *CWSConsumer
	policiesStatusLock sync.RWMutex
	policiesStatus     []*api.PolicyStatus
	msgSender          MsgSender
	connEstablished    *atomic.Bool
	envAsTags          []string

	// os release data
	kernelVersion string
	distribution  string

	stopChan chan struct{}
	stopper  startstop.Stopper
}

// GetActivityDumpStream waits for activity dumps and forwards them to the stream
func (a *APIServer) GetActivityDumpStream(_ *api.ActivityDumpStreamParams, stream api.SecurityModule_GetActivityDumpStreamServer) error {
	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-a.stopChan:
			return nil
		case dump := <-a.activityDumps:
			if err := stream.Send(dump); err != nil {
				return err
			}
		}
	}
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

// GetEvents waits for security events
func (a *APIServer) GetEvents(_ *api.GetEventParams, stream api.SecurityModule_GetEventsServer) error {
	if prev := a.connEstablished.Swap(true); !prev {
		// should always be non nil
		if a.cwsConsumer != nil {
			a.cwsConsumer.onAPIConnectionEstablished()
		}
	}

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-a.stopChan:
			return nil
		case msg := <-a.msgs:
			if err := stream.Send(msg); err != nil {
				return err
			}
		}
	}
}

func (a *APIServer) enqueue(msg *pendingMsg) {
	a.queueLock.Lock()
	a.queue = append(a.queue, msg)
	a.queueLock.Unlock()
}

func (a *APIServer) dequeue(now time.Time, cb func(msg *pendingMsg) bool) {
	a.queueLock.Lock()
	defer a.queueLock.Unlock()

	a.queue = slicesDeleteUntilFalse(a.queue, func(msg *pendingMsg) bool {
		if msg.sendAfter.After(now) {
			return false
		}

		if cb(msg) {
			return true
		}

		if msg.retry >= maxRetry {
			seclog.Errorf("failed to sent event, max retry reached: %d", msg.retry)
			return true
		}
		seclog.Tracef("failed to sent event, retry %d/%d", msg.retry, maxRetry)

		msg.sendAfter = now.Add(retryDelay)
		msg.retry++

		return false
	})
}

// slicesDeleteUntilFalse deletes elements from the slice until the function f returns false.
func slicesDeleteUntilFalse(s []*pendingMsg, f func(*pendingMsg) bool) []*pendingMsg {
	for i, v := range s {
		if !f(v) {
			return s[i:]
		}
	}

	return nil
}

func (a *APIServer) updateMsgService(msg *api.SecurityEventMessage) {
	// look for the service tag if we don't have one yet
	if len(msg.Service) == 0 {
		for _, tag := range msg.Tags {
			if strings.HasPrefix(tag, "service:") {
				msg.Service = strings.TrimPrefix(tag, "service:")
				break
			}
		}
	}
}

func (a *APIServer) updateCustomEventTags(msg *api.SecurityEventMessage) {
	appendTagsIfNotPresent := func(toAdd []string) {
		for _, tag := range toAdd {
			key, _, _ := strings.Cut(tag, ":")
			if !slices.ContainsFunc(msg.Tags, func(t string) bool {
				return strings.HasPrefix(t, key+":")
			}) {
				msg.Tags = append(msg.Tags, tag)
			}
		}
	}

	// on fargate, append global tags on custom events
	if fargate.IsFargateInstance() {
		appendTagsIfNotPresent(a.getGlobalTags())
	}

	// add agent tags on custom events
	acc := a.probe.GetAgentContainerContext()
	if acc != nil && acc.ContainerID != "" {
		appendTagsIfNotPresent(a.probe.GetEventTags(acc.ContainerID))
	}
}

func (a *APIServer) start(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case now := <-ticker.C:
			a.dequeue(now, func(msg *pendingMsg) bool {
				if msg.extTagsCb != nil {
					tags, retryable := msg.extTagsCb()
					if len(tags) == 0 && retryable && msg.retry < maxRetry {
						return false
					}

					// dedup
					for _, tag := range tags {
						if !slices.Contains(msg.tags, tag) {
							msg.tags = append(msg.tags, tag)
						}
					}
				}

				// not fully resolved, retry
				if !msg.isResolved() && msg.retry < maxRetry {
					return false
				}

				data, err := msg.toJSON()
				if err != nil {
					seclog.Errorf("failed to marshal event context: %v", err)
					return true
				}

				seclog.Tracef("Sending event message for rule `%s` to security-agent `%s`", msg.ruleID, string(data))

				m := &api.SecurityEventMessage{
					RuleID:    msg.ruleID,
					Data:      data,
					Service:   msg.service,
					Tags:      msg.tags,
					Timestamp: timestamppb.New(msg.timestamp),
				}
				a.updateMsgService(m)

				a.msgSender.Send(m, a.expireEvent)

				return true
			})
		case <-ctx.Done():
			close(a.stopChan)
			return
		}
	}
}

// Start the api server, starts to consume the msg queue
func (a *APIServer) Start(ctx context.Context) {
	go a.start(ctx)
}

// GetConfig returns config of the runtime security module required by the security agent
func (a *APIServer) GetConfig(_ context.Context, _ *api.GetConfigParams) (*api.SecurityConfigMessage, error) {
	if a.cfg != nil {
		return &api.SecurityConfigMessage{
			FIMEnabled:          a.cfg.FIMEnabled,
			RuntimeEnabled:      a.cfg.RuntimeEnabled,
			ActivityDumpEnabled: a.probe.IsActivityDumpEnabled(),
		}, nil
	}
	return &api.SecurityConfigMessage{}, nil
}

// SendEvent forwards events sent by the runtime security module to Datadog
func (a *APIServer) SendEvent(rule *rules.Rule, event events.Event, extTagsCb func() ([]string, bool), service string) {
	backendEvent := events.BackendEvent{
		Title: rule.Def.Description,
		AgentContext: events.AgentContext{
			RuleID:        rule.Def.ID,
			RuleVersion:   rule.Def.Version,
			Version:       version.AgentVersion,
			OS:            runtime.GOOS,
			Arch:          utils.RuntimeArch(),
			Origin:        a.probe.Origin(),
			KernelVersion: a.kernelVersion,
			Distribution:  a.distribution,
			PolicyName:    rule.Policy.Name,
			PolicyVersion: rule.Policy.Version,
		},
	}

	seclog.Tracef("Prepare event message for rule `%s`", rule.ID)

	// no retention if there is no ext tags to resolve
	retention := a.retention
	if extTagsCb == nil {
		retention = 0
	}

	ruleID := rule.Def.ID
	if rule.Def.GroupID != "" {
		ruleID = rule.Def.GroupID
	}

	// get type tags + container tags if already resolved, see ResolveContainerTags
	eventTags := event.GetTags()

	tags := []string{"rule_id:" + ruleID}
	tags = append(tags, rule.Tags...)
	tags = append(tags, eventTags...)
	tags = append(tags, common.QueryAccountIDTag())
	tags = append(tags, a.envAsTags...)

	// model event or custom event ? if model event use queuing so that tags and actions can be handled
	if ev, ok := event.(*model.Event); ok {
		eventActionReports := ev.GetActionReports()
		actionReports := make([]model.ActionReport, 0, len(eventActionReports))
		for _, ar := range eventActionReports {
			if ar.IsMatchingRule(rule.ID) {
				actionReports = append(actionReports, ar)
			}
		}

		timestamp := ev.ResolveEventTime()
		if timestamp.IsZero() {
			timestamp = time.Now()
		}

		msg := &pendingMsg{
			ruleID:          ruleID,
			backendEvent:    backendEvent,
			eventSerializer: serializers.NewEventSerializer(ev, rule),
			extTagsCb:       extTagsCb,
			service:         service,
			timestamp:       timestamp,
			sendAfter:       time.Now().Add(retention),
			tags:            tags,
			actionReports:   actionReports,
		}

		a.enqueue(msg)
	} else {
		var (
			backendEventJSON []byte
			eventJSON        []byte
			err              error
		)
		backendEventJSON, err = easyjson.Marshal(backendEvent)
		if err != nil {
			seclog.Errorf("failed to marshal event: %v", err)
		}

		if ev, ok := event.(events.EventMarshaler); ok {
			if eventJSON, err = ev.ToJSON(); err != nil {
				seclog.Errorf("failed to marshal event: %v", err)
				return
			}
		} else {
			if eventJSON, err = json.Marshal(event); err != nil {
				seclog.Errorf("failed to marshal event: %v : %+v", err, event)
				return
			}
		}

		data, err := mergeJSON(backendEventJSON, eventJSON)
		if err != nil {
			seclog.Errorf("failed to merge event json: %v", err)
			return
		}

		seclog.Tracef("Sending event message for rule `%s` to security-agent `%s`", ruleID, string(data))

		// for custom events, we can use the current time as timestamp
		timestamp := time.Now()

		m := &api.SecurityEventMessage{
			RuleID:    ruleID,
			Data:      data,
			Service:   service,
			Tags:      tags,
			Timestamp: timestamppb.New(timestamp),
		}
		a.updateCustomEventTags(m)
		a.updateMsgService(m)

		a.msgSender.Send(m, a.expireEvent)
	}
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
	// update metric
	a.expiredDumps.Inc()

	selectorStr := "<unknown>"
	if sel := dump.GetSelector(); sel != nil {
		selectorStr = fmt.Sprintf("%s:%s", sel.GetName(), sel.GetTag())
	}
	seclog.Tracef("the activity dump server channel is full, a dump of [%s] was dropped\n", selectorStr)
}

// GetStats returns a map indexed by ruleIDs that describes the amount of events
// that were expired or rate limited before reaching
func (a *APIServer) GetStats() map[string]int64 {
	a.expiredEventsLock.RLock()
	defer a.expiredEventsLock.RUnlock()

	stats := make(map[string]int64, len(a.expiredEvents))
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
	return nil
}

// ReloadPolicies reloads the policies
func (a *APIServer) ReloadPolicies(_ context.Context, _ *api.ReloadPoliciesParams) (*api.ReloadPoliciesResultMessage, error) {
	if a.cwsConsumer == nil || a.cwsConsumer.ruleEngine == nil {
		return nil, errors.New("no rule engine")
	}

	if err := a.cwsConsumer.ruleEngine.ReloadPolicies(); err != nil {
		return nil, err
	}
	return &api.ReloadPoliciesResultMessage{}, nil
}

// GetRuleSetReport reports the ruleset loaded
func (a *APIServer) GetRuleSetReport(_ context.Context, _ *api.GetRuleSetReportParams) (*api.GetRuleSetReportMessage, error) {
	if a.cwsConsumer == nil || a.cwsConsumer.ruleEngine == nil {
		return nil, errors.New("no rule engine")
	}

	ruleSet := a.cwsConsumer.ruleEngine.GetRuleSet()
	if ruleSet == nil {
		return nil, fmt.Errorf("failed to get loaded rule set")
	}

	cfg := &pconfig.Config{
		EnableKernelFilters: a.probe.Config.Probe.EnableKernelFilters,
		EnableApprovers:     a.probe.Config.Probe.EnableApprovers,
		EnableDiscarders:    a.probe.Config.Probe.EnableDiscarders,
		PIDCacheSize:        a.probe.Config.Probe.PIDCacheSize,
	}

	report, err := kfilters.ComputeFilters(cfg, ruleSet)
	if err != nil {
		return nil, err
	}

	return &api.GetRuleSetReportMessage{
		RuleSetReportMessage: api.FromFilterReportToProtoRuleSetReportMessage(report),
	}, nil
}

// ApplyRuleIDs the rule ids
func (a *APIServer) ApplyRuleIDs(ruleIDs []rules.RuleID) {
	a.expiredEventsLock.Lock()
	defer a.expiredEventsLock.Unlock()

	a.expiredEvents = make(map[rules.RuleID]*atomic.Int64)
	for _, id := range ruleIDs {
		a.expiredEvents[id] = atomic.NewInt64(0)
	}
}

// ApplyPolicyStates the policy states
func (a *APIServer) ApplyPolicyStates(policies []*monitor.PolicyState) {
	a.policiesStatusLock.Lock()
	defer a.policiesStatusLock.Unlock()

	a.policiesStatus = []*api.PolicyStatus{}
	for _, policy := range policies {
		entry := api.PolicyStatus{
			Name:   policy.Name,
			Source: policy.Source,
		}

		for _, rule := range policy.Rules {
			entry.Status = append(entry.Status, &api.RuleStatus{
				ID:     rule.ID,
				Status: rule.Status,
				Error:  rule.Message,
			})
		}

		a.policiesStatus = append(a.policiesStatus, &entry)
	}
}

// GetSECLVariables returns the SECL variables and their value
func (a *APIServer) GetSECLVariables() map[string]*api.SECLVariableState {
	return a.cwsConsumer.ruleEngine.GetSECLVariables()
}

// Stop stops the API server
func (a *APIServer) Stop() {
	a.stopper.Stop()
}

// SetCWSConsumer sets the CWS consumer
func (a *APIServer) SetCWSConsumer(consumer *CWSConsumer) {
	a.cwsConsumer = consumer
}

func (a *APIServer) getGlobalTags() []string {
	tagger := a.probe.Opts.Tagger

	if tagger == nil {
		return nil
	}

	globalTags, err := tagger.GlobalTags(types.OrchestratorCardinality)
	if err != nil {
		seclog.Errorf("failed to get global tags: %v", err)
		return nil
	}
	return globalTags
}

func getEnvAsTags(cfg *config.RuntimeSecurityConfig) []string {
	tags := []string{}

	for _, env := range cfg.EnvAsTags {
		value := os.Getenv(env)
		if value != "" {
			tags = append(tags, fmt.Sprintf("%s:%s", env, value))
		}
	}
	return tags
}

// NewAPIServer returns a new gRPC event server
func NewAPIServer(cfg *config.RuntimeSecurityConfig, probe *sprobe.Probe, msgSender MsgSender, client statsd.ClientInterface, selfTester *selftests.SelfTester, compression compression.Component, ipc ipc.Component) (*APIServer, error) {
	stopper := startstop.NewSerialStopper()

	as := &APIServer{
		msgs:            make(chan *api.SecurityEventMessage, cfg.EventServerBurst*3),
		activityDumps:   make(chan *api.ActivityDumpStreamMessage, model.MaxTracedCgroupsCount*2),
		expiredEvents:   make(map[rules.RuleID]*atomic.Int64),
		expiredDumps:    atomic.NewInt64(0),
		statsdClient:    client,
		probe:           probe,
		retention:       cfg.EventServerRetention,
		cfg:             cfg,
		stopper:         stopper,
		selfTester:      selfTester,
		stopChan:        make(chan struct{}),
		msgSender:       msgSender,
		connEstablished: atomic.NewBool(false),
		envAsTags:       getEnvAsTags(cfg),
	}

	as.collectOSReleaseData()

	if as.msgSender == nil {
		if cfg.SendEventFromSystemProbe {
			msgSender, err := NewDirectMsgSender(stopper, compression, ipc)
			if err != nil {
				log.Errorf("failed to setup direct reporter: %v", err)
			} else {
				as.msgSender = msgSender
			}
		}

		if as.msgSender == nil {
			as.msgSender = NewChanMsgSender(as.msgs)
		}
	}

	return as, nil
}

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
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/mailru/easyjson"
	"go.uber.org/atomic"

	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
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
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	maxRetry   = 3
	retryDelay = time.Second
)

type pendingMsg struct {
	ruleID        string
	backendEvent  events.BackendEvent
	eventJSON     []byte
	tags          []string
	actionReports []model.ActionReport
	service       string
	extTagsCb     func() []string
	sendAfter     time.Time
	retry         int
}

func (p *pendingMsg) ToJSON() ([]byte, bool, error) {
	fullyResolved := true

	p.backendEvent.RuleActions = []json.RawMessage{}

	for _, report := range p.actionReports {
		data, resolved, err := report.ToJSON()
		if err != nil {
			return nil, false, err
		}
		p.backendEvent.RuleActions = append(p.backendEvent.RuleActions, data)

		if !resolved {
			fullyResolved = false
		}
	}

	backendEventJSON, err := easyjson.Marshal(p.backendEvent)
	if err != nil {
		return nil, false, err
	}

	data := append(p.eventJSON[:len(p.eventJSON)-1], ',')
	data = append(data, backendEventJSON[1:]...)

	return data, fullyResolved, nil
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

	a.queue = slices.DeleteFunc(a.queue, func(msg *pendingMsg) bool {
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
		seclog.Debugf("failed to sent event, retry %d/%d", msg.retry, maxRetry)

		msg.sendAfter = now.Add(retryDelay)
		msg.retry++

		return false
	})
}

func (a *APIServer) start(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case now := <-ticker.C:
			a.dequeue(now, func(msg *pendingMsg) bool {
				if msg.extTagsCb != nil {
					// dedup
					for _, tag := range msg.extTagsCb() {
						if !slices.Contains(msg.tags, tag) {
							msg.tags = append(msg.tags, tag)
						}
					}
				}

				// recopy tags
				hasService := len(msg.service) != 0
				for _, tag := range msg.tags {
					// look for the service tag if we don't have one yet
					if !hasService {
						if strings.HasPrefix(tag, "service:") {
							msg.service = strings.TrimPrefix(tag, "service:")
							hasService = true
						}
					}
				}

				data, resolved, err := msg.ToJSON()
				if err != nil {
					seclog.Errorf("failed to marshal event context: %v", err)
					return true
				}

				// not fully resolved, retry
				if !resolved && msg.retry < maxRetry {
					return false
				}

				seclog.Tracef("Sending event message for rule `%s` to security-agent `%s`", msg.ruleID, string(data))

				m := &api.SecurityEventMessage{
					RuleID:  msg.ruleID,
					Data:    data,
					Service: msg.service,
					Tags:    msg.tags,
				}

				a.msgSender.Send(m, a.expireEvent)

				return true
			})
		case <-ctx.Done():
			a.stopChan <- struct{}{}
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
func (a *APIServer) SendEvent(rule *rules.Rule, e events.Event, extTagsCb func() []string, service string) {
	backendEvent := events.BackendEvent{
		Title: rule.Definition.Description,
		AgentContext: events.AgentContext{
			RuleID:      rule.Definition.ID,
			RuleVersion: rule.Definition.Version,
			Version:     version.AgentVersion,
			OS:          runtime.GOOS,
			Arch:        utils.RuntimeArch(),
			Origin:      a.probe.Origin(),
		},
	}

	if policy := rule.Definition.Policy; policy != nil {
		backendEvent.AgentContext.PolicyName = policy.Name
		backendEvent.AgentContext.PolicyVersion = policy.Version
	}

	eventJSON, err := marshalEvent(e, rule.Opts)
	if err != nil {
		seclog.Errorf("failed to marshal event: %v", err)
		return
	}

	seclog.Tracef("Prepare event message for rule `%s` : `%s`", rule.ID, string(eventJSON))

	// no retention if there is no ext tags to resolve
	retention := a.retention
	if extTagsCb == nil {
		retention = 0
	}

	// get type tags + container tags if already resolved, see ResolveContainerTags
	eventTags := e.GetTags()

	ruleID := rule.Definition.ID
	if rule.Definition.GroupID != "" {
		ruleID = rule.Definition.GroupID
	}

	msg := &pendingMsg{
		ruleID:        ruleID,
		backendEvent:  backendEvent,
		eventJSON:     eventJSON,
		extTagsCb:     extTagsCb,
		service:       service,
		sendAfter:     time.Now().Add(retention),
		tags:          make([]string, 0, 1+len(rule.Tags)+len(eventTags)+1),
		actionReports: e.GetActionReports(),
	}

	msg.tags = append(msg.tags, "rule_id:"+ruleID)
	msg.tags = append(msg.tags, rule.Tags...)
	msg.tags = append(msg.tags, eventTags...)
	msg.tags = append(msg.tags, common.QueryAccountIDTag())

	a.enqueue(msg)
}

func marshalEvent(event events.Event, opts *eval.Opts) ([]byte, error) {
	if ev, ok := event.(*model.Event); ok {
		return serializers.MarshalEvent(ev, opts)
	}

	if ev, ok := event.(events.EventMarshaler); ok {
		return ev.ToJSON()
	}

	return json.Marshal(event)
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
	seclog.Tracef("the activity dump server channel is full, a dump of [%s] was dropped\n", dump.GetDump().GetMetadata().GetName())
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
func (a *APIServer) GetRuleSetReport(_ context.Context, _ *api.GetRuleSetReportParams) (*api.GetRuleSetReportResultMessage, error) {
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

	report, err := kfilters.NewApplyRuleSetReport(cfg, ruleSet)
	if err != nil {
		return nil, err
	}

	return &api.GetRuleSetReportResultMessage{
		RuleSetReportMessage: api.FromKFiltersToProtoRuleSetReport(report),
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

// Stop stops the API server
func (a *APIServer) Stop() {
	a.stopper.Stop()
}

// SetCWSConsumer sets the CWS consumer
func (a *APIServer) SetCWSConsumer(consumer *CWSConsumer) {
	a.cwsConsumer = consumer
}

// NewAPIServer returns a new gRPC event server
func NewAPIServer(cfg *config.RuntimeSecurityConfig, probe *sprobe.Probe, msgSender MsgSender, client statsd.ClientInterface, selfTester *selftests.SelfTester) *APIServer {
	stopper := startstop.NewSerialStopper()

	as := &APIServer{
		msgs:          make(chan *api.SecurityEventMessage, cfg.EventServerBurst*3),
		activityDumps: make(chan *api.ActivityDumpStreamMessage, model.MaxTracedCgroupsCount*2),
		expiredEvents: make(map[rules.RuleID]*atomic.Int64),
		expiredDumps:  atomic.NewInt64(0),
		statsdClient:  client,
		probe:         probe,
		retention:     cfg.EventServerRetention,
		cfg:           cfg,
		stopper:       stopper,
		selfTester:    selfTester,
		stopChan:      make(chan struct{}),
		msgSender:     msgSender,
	}

	if as.msgSender == nil {
		if pkgconfig.SystemProbe.GetBool("runtime_security_config.direct_send_from_system_probe") {
			msgSender, err := NewDirectMsgSender(stopper)
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

	return as
}

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
	"strings"
	"sync"
	"time"

	pconfig "github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"

	"github.com/DataDog/datadog-go/v5/statsd"
	easyjson "github.com/mailru/easyjson"
	jwriter "github.com/mailru/easyjson/jwriter"
	"go.uber.org/atomic"
	"golang.org/x/time/rate"

	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/selftests"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/reporter"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type pendingMsg struct {
	ruleID    string
	data      []byte
	tags      []string
	service   string
	extTagsCb func() []string
	sendAfter time.Time
}

// APIServer represents a gRPC server in charge of receiving events sent by
// the runtime security system-probe module and forwards them to Datadog
type APIServer struct {
	api.UnimplementedSecurityModuleServer
	msgs              chan *api.SecurityEventMessage
	directReporter    common.RawReporter
	activityDumps     chan *api.ActivityDumpStreamMessage
	expiredEventsLock sync.RWMutex
	expiredEvents     map[rules.RuleID]*atomic.Int64
	expiredDumps      *atomic.Int64
	limiter           *events.StdLimiter
	statsdClient      statsd.ClientInterface
	probe             *sprobe.Probe
	queueLock         sync.Mutex
	queue             []*pendingMsg
	retention         time.Duration
	cfg               *config.RuntimeSecurityConfig
	selfTester        *selftests.SelfTester
	cwsConsumer       *CWSConsumer

	stopper startstop.Stopper
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

// GetEvents waits for security events
func (a *APIServer) GetEvents(params *api.GetEventParams, stream api.SecurityModule_GetEventsServer) error {
	// Read 10 security events per call
	msgs := 10
LOOP:
	for {
		// Check that the limit is not reached
		if !a.limiter.Allow(nil) {
			return nil
		}

		// Read one message
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

// RuleEvent is a wrapper used to send an event to the backend
type RuleEvent struct {
	RuleID string       `json:"rule_id"`
	Event  events.Event `json:"event"`
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
				if msg.extTagsCb != nil {
					msg.tags = append(msg.tags, msg.extTagsCb()...)
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

				m := &api.SecurityEventMessage{
					RuleID:  msg.ruleID,
					Data:    msg.data,
					Service: msg.service,
					Tags:    msg.tags,
				}

				if a.directReporter != nil {
					a.sendDirectly(m)
				} else {
					a.sendToSecurityAgent(m)
				}
			})
		case <-ctx.Done():
			return
		}
	}
}

func (a *APIServer) sendToSecurityAgent(m *api.SecurityEventMessage) {
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
}

func (a *APIServer) sendDirectly(m *api.SecurityEventMessage) {
	a.directReporter.ReportRaw(m.Data, m.Service, m.Tags...)
}

// Start the api server, starts to consume the msg queue
func (a *APIServer) Start(ctx context.Context) {
	go a.start(ctx)
}

// GetConfig returns config of the runtime security module required by the security agent
func (a *APIServer) GetConfig(ctx context.Context, params *api.GetConfigParams) (*api.SecurityConfigMessage, error) {
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
	agentContext := events.AgentContext{
		RuleID:      rule.Definition.ID,
		RuleVersion: rule.Definition.Version,
		Version:     version.AgentVersion,
	}

	ruleEvent := &events.Signal{
		Title:        rule.Definition.Description,
		AgentContext: agentContext,
	}

	if policy := rule.Definition.Policy; policy != nil {
		ruleEvent.AgentContext.PolicyName = policy.Name
		ruleEvent.AgentContext.PolicyVersion = policy.Version
	}

	probeJSON, err := marshalEvent(e, a.probe)
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

	eventTags := e.GetTags()
	msg := &pendingMsg{
		ruleID:    rule.Definition.ID,
		data:      data,
		extTagsCb: extTagsCb,
		service:   service,
		sendAfter: time.Now().Add(a.retention),
		tags:      make([]string, 0, 1+len(rule.Tags)+len(eventTags)+1),
	}

	msg.tags = append(msg.tags, "rule_id:"+rule.Definition.ID)
	msg.tags = append(msg.tags, rule.Tags...)
	msg.tags = append(msg.tags, eventTags...)
	msg.tags = append(msg.tags, common.QueryAccountIDTag())

	a.enqueue(msg)
}

func marshalEvent(event events.Event, probe *sprobe.Probe) ([]byte, error) {
	if ev, ok := event.(*model.Event); ok {
		return serializers.MarshalEvent(ev, probe.GetResolvers())
	}

	if m, ok := event.(easyjson.Marshaler); ok {
		w := &jwriter.Writer{
			Flags: jwriter.NilSliceAsEmpty | jwriter.NilMapAsEmpty,
		}
		m.MarshalEasyJSON(w)
		return w.BuildBytes()
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
func (a *APIServer) ReloadPolicies(ctx context.Context, params *api.ReloadPoliciesParams) (*api.ReloadPoliciesResultMessage, error) {
	if a.cwsConsumer == nil || a.cwsConsumer.ruleEngine == nil {
		return nil, errors.New("no rule engine")
	}

	if err := a.cwsConsumer.ruleEngine.ReloadPolicies(); err != nil {
		return nil, err
	}
	return &api.ReloadPoliciesResultMessage{}, nil
}

// GetRuleSetReport reports the ruleset loaded
func (a *APIServer) GetRuleSetReport(ctx context.Context, params *api.GetRuleSetReportParams) (*api.GetRuleSetReportResultMessage, error) {
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

// Apply a rule set
func (a *APIServer) Apply(ruleIDs []rules.RuleID) {
	a.expiredEventsLock.Lock()
	defer a.expiredEventsLock.Unlock()

	a.expiredEvents = make(map[rules.RuleID]*atomic.Int64)
	for _, id := range ruleIDs {
		a.expiredEvents[id] = atomic.NewInt64(0)
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
func NewAPIServer(cfg *config.RuntimeSecurityConfig, probe *sprobe.Probe, client statsd.ClientInterface, selfTester *selftests.SelfTester) *APIServer {
	stopper := startstop.NewSerialStopper()
	directReporter, err := newDirectReporter(stopper)
	if err != nil {
		log.Errorf("failed to setup direct reporter: %v", err)
		directReporter = nil
	}

	es := &APIServer{
		msgs:           make(chan *api.SecurityEventMessage, cfg.EventServerBurst*3),
		directReporter: directReporter,
		activityDumps:  make(chan *api.ActivityDumpStreamMessage, model.MaxTracedCgroupsCount*2),
		expiredEvents:  make(map[rules.RuleID]*atomic.Int64),
		expiredDumps:   atomic.NewInt64(0),
		limiter:        events.NewStdLimiter(rate.Limit(cfg.EventServerRate), cfg.EventServerBurst),
		statsdClient:   client,
		probe:          probe,
		retention:      cfg.EventServerRetention,
		cfg:            cfg,
		stopper:        stopper,
		selfTester:     selfTester,
	}
	return es
}

func newDirectReporter(stopper startstop.Stopper) (common.RawReporter, error) {
	directReportEnabled := pkgconfig.SystemProbe.GetBool("runtime_security_config.direct_send_from_system_probe")
	if !directReportEnabled {
		return nil, nil
	}

	runPath := pkgconfig.Datadog.GetString("runtime_security_config.run_path")
	useSecRuntimeTrack := pkgconfig.SystemProbe.GetBool("runtime_security_config.use_secruntime_track")

	endpoints, destinationsCtx, err := common.NewLogContextRuntime(useSecRuntimeTrack)
	if err != nil {
		return nil, fmt.Errorf("failed to create direct reported endpoints: %w", err)
	}

	for _, status := range endpoints.GetStatus() {
		log.Info(status)
	}

	reporter, err := reporter.NewCWSReporter(runPath, stopper, endpoints, destinationsCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to create direct reporter: %w", err)
	}

	return reporter, nil
}

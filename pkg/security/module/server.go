// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package module

import (
	"context"
	json "encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	easyjson "github.com/mailru/easyjson"
	jwriter "github.com/mailru/easyjson/jwriter"
	"go.uber.org/atomic"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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

// APIServer represents a gRPC server in charge of receiving events sent by
// the runtime security system-probe module and forwards them to Datadog
type APIServer struct {
	api.UnimplementedSecurityModuleServer
	msgs              chan *api.SecurityEventMessage
	activityDumps     chan *api.ActivityDumpStreamMessage
	expiredEventsLock sync.RWMutex
	expiredEvents     map[rules.RuleID]*atomic.Int64
	expiredDumpsLock  sync.RWMutex
	expiredDumps      *atomic.Int64
	rate              *Limiter
	statsdClient      statsd.ClientInterface
	probe             *sprobe.Probe
	queueLock         sync.Mutex
	queue             []*pendingMsg
	retention         time.Duration
	cfg               *config.RuntimeSecurityConfig
	cwsConsumer       *CWSConsumer
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
		if !a.rate.limiter.Allow() {
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
	RuleID string `json:"rule_id"`
	Event  Event  `json:"event"`
}

// DumpDiscarders handles discarder dump requests
func (a *APIServer) DumpDiscarders(ctx context.Context, params *api.DumpDiscardersParams) (*api.DumpDiscardersMessage, error) {
	filePath, err := a.probe.DumpDiscarders()
	if err != nil {
		return nil, err
	}
	seclog.Infof("Discarder dump file path: %s", filePath)

	return &api.DumpDiscardersMessage{DumpFilename: filePath}, nil
}

// DumpProcessCache handles process cache dump requests
func (a *APIServer) DumpProcessCache(ctx context.Context, params *api.DumpProcessCacheParams) (*api.SecurityDumpProcessCacheMessage, error) {
	resolvers := a.probe.GetResolvers()

	filename, err := resolvers.ProcessResolver.Dump(params.WithArgs)
	if err != nil {
		return nil, err
	}

	return &api.SecurityDumpProcessCacheMessage{
		Filename: filename,
	}, nil
}

// DumpActivity handle an activity dump request
func (a *APIServer) DumpActivity(ctx context.Context, params *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	if handler := a.probe.GetProfileHandler(); handler != nil {
		msg, err := handler.DumpActivity(params)
		if err != nil {
			seclog.Errorf(err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// ListActivityDumps returns the list of active dumps
func (a *APIServer) ListActivityDumps(ctx context.Context, params *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error) {
	if handler := a.probe.GetProfileHandler(); handler != nil {
		msg, err := handler.ListActivityDumps(params)
		if err != nil {
			seclog.Errorf(err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// StopActivityDump stops an active activity dump if it exists
func (a *APIServer) StopActivityDump(ctx context.Context, params *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	if handler := a.probe.GetProfileHandler(); handler != nil {
		msg, err := handler.StopActivityDump(params)
		if err != nil {
			seclog.Errorf(err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// TranscodingRequest encodes an activity dump following the requested parameters
func (a *APIServer) TranscodingRequest(ctx context.Context, params *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	if handler := a.probe.GetProfileHandler(); handler != nil {
		msg, err := handler.GenerateTranscoding(params)
		if err != nil {
			seclog.Errorf(err.Error())
		}
		return msg, nil
	}

	return nil, fmt.Errorf("monitor not configured")
}

// GetStatus returns the status of the module
func (a *APIServer) GetStatus(ctx context.Context, params *api.GetStatusParams) (*api.Status, error) {
	status, err := a.probe.GetConstantFetcherStatus()
	if err != nil {
		return nil, err
	}

	constants := make([]*api.ConstantValueAndSource, 0, len(status.Values))
	for _, v := range status.Values {
		constants = append(constants, &api.ConstantValueAndSource{
			ID:     v.ID,
			Value:  v.Value,
			Source: v.FetcherName,
		})
	}

	apiStatus := &api.Status{
		Environment: &api.EnvironmentStatus{
			Constants: &api.ConstantFetcherStatus{
				Fetchers: status.Fetchers,
				Values:   constants,
			},
		},
		SelfTests: a.cwsConsumer.selfTester.GetStatus(),
	}

	envErrors := a.probe.VerifyEnvironment()
	if envErrors != nil {
		apiStatus.Environment.Warnings = make([]string, len(envErrors.Errors))
		for i, err := range envErrors.Errors {
			apiStatus.Environment.Warnings[i] = err.Error()
		}
	}

	apiStatus.Environment.KernelLockdown = string(kernel.GetLockdownMode())

	if kernel, err := a.probe.GetKernelVersion(); err == nil {
		apiStatus.Environment.UseMmapableMaps = kernel.HaveMmapableMaps()
		apiStatus.Environment.UseRingBuffer = a.probe.UseRingBuffers()
	}

	return apiStatus, nil
}

// DumpNetworkNamespace handles network namespace cache dump requests
func (a *APIServer) DumpNetworkNamespace(ctx context.Context, params *api.DumpNetworkNamespaceParams) (*api.DumpNetworkNamespaceMessage, error) {
	return a.probe.GetResolvers().NamespaceResolver.DumpNetworkNamespaces(params), nil
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
			FIMEnabled:          a.cfg.FIMEnabled,
			RuntimeEnabled:      a.cfg.RuntimeEnabled,
			ActivityDumpEnabled: a.cfg.IsActivityDumpEnabled(),
		}, nil
	}
	return &api.SecurityConfigMessage{}, nil
}

// RunSelfTest runs self test and then reload the current policies
func (a *APIServer) RunSelfTest(ctx context.Context, params *api.RunSelfTestParams) (*api.SecuritySelfTestResultMessage, error) {
	if a.cwsConsumer == nil {
		return nil, errors.New("failed to found module in APIServer")
	}

	if a.cwsConsumer.selfTester == nil {
		return &api.SecuritySelfTestResultMessage{
			Ok:    false,
			Error: "self-tests are disabled",
		}, nil
	}

	if _, err := a.cwsConsumer.RunSelfTest(false); err != nil {
		return &api.SecuritySelfTestResultMessage{
			Ok:    false,
			Error: err.Error(),
		}, nil
	}

	return &api.SecuritySelfTestResultMessage{
		Ok:    true,
		Error: "",
	}, nil
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

	probeJSON, err := marshalEvent(event, a.probe)
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

func marshalEvent(event Event, probe *sprobe.Probe) ([]byte, error) {
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
	return nil
}

// ReloadPolicies reloads the policies
func (a *APIServer) ReloadPolicies(ctx context.Context, params *api.ReloadPoliciesParams) (*api.ReloadPoliciesResultMessage, error) {
	if err := a.cwsConsumer.ReloadPolicies(); err != nil {
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

// NewAPIServer returns a new gRPC event server
func NewAPIServer(cfg *config.RuntimeSecurityConfig, probe *sprobe.Probe, client statsd.ClientInterface) *APIServer {
	es := &APIServer{
		msgs:          make(chan *api.SecurityEventMessage, cfg.EventServerBurst*3),
		activityDumps: make(chan *api.ActivityDumpStreamMessage, model.MaxTracedCgroupsCount*2),
		expiredEvents: make(map[rules.RuleID]*atomic.Int64),
		expiredDumps:  atomic.NewInt64(0),
		rate:          NewLimiter(rate.Limit(cfg.EventServerRate), cfg.EventServerBurst),
		statsdClient:  client,
		probe:         probe,
		retention:     time.Duration(cfg.EventServerRetention) * time.Second,
		cfg:           cfg,
	}
	return es
}

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
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/mailru/easyjson"
	"go.uber.org/atomic"
	empty "google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

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
	"github.com/DataDog/datadog-agent/pkg/security/proto/api/transform"
	"github.com/DataDog/datadog-agent/pkg/security/rules/monitor"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// defaultMaxRetry is the default maximum number of retries for a pending message
	defaultMaxRetry = 5

	// retryDelay is the delay between retries, changing this value may impact the retry logic.
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

	sshSessionPatcher sshSessionPatcher
}

func (p *pendingMsg) getMaxRetry() int {
	maxRetry := defaultMaxRetry
	for _, report := range p.actionReports {
		maxRetry = max(maxRetry, report.MaxRetry())
	}
	if p.sshSessionPatcher != nil {
		maxRetry = max(maxRetry, p.sshSessionPatcher.MaxRetry())
	}
	return maxRetry
}

func (p *pendingMsg) isResolved() bool {
	for _, report := range p.actionReports {
		if err := report.IsResolved(); err != nil {
			seclog.Debugf("action report not resolved: %v", err)
			return false
		}
	}

	// TODO: for now skip the retry mechanism and always send the event
	// if p.sshSessionPatcher != nil {
	// 	if err := p.sshSessionPatcher.IsResolved(); err != nil {
	// 		seclog.Tracef("ssh session not resolved: %v", err)
	// 		return false
	// 	}
	// }
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

	if p.sshSessionPatcher != nil {
		p.sshSessionPatcher.PatchEvent(p.eventSerializer)
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
	api.UnimplementedSecurityModuleEventServer
	api.UnimplementedSecurityModuleCmdServer
	events             chan *api.SecurityEventMessage
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
	msgSender          EventMsgSender
	activityDumpSender ActivityDumpMsgSender
	connEstablished    *atomic.Bool
	envAsTags          []string
	containerFilter    *containers.Filter

	// os release data
	kernelVersion string
	distribution  string

	stopChan chan struct{}
	stopper  startstop.Stopper
	wg       sync.WaitGroup

	securityAgentAPIClient *SecurityAgentAPIClient
}

// GetActivityDumpStream transfers dumps to the security-agent. Communication security-agent -> system-probe
func (a *APIServer) GetActivityDumpStream(_ *empty.Empty, stream api.SecurityModuleEvent_GetActivityDumpStreamServer) error {
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
func (a *APIServer) SendActivityDump(imageName string, imageTag string, header []byte, data []byte) {
	dump := &api.ActivityDumpStreamMessage{
		Selector: &api.WorkloadSelectorMessage{
			Name: imageName,
			Tag:  imageTag,
		},
		Header: header,
		Data:   data,
	}

	a.activityDumpSender.Send(dump, a.expireDump)
}

// GetEventStream transfers events to the security-agent. Communication security-agent -> system-probe
func (a *APIServer) GetEventStream(_ *empty.Empty, stream api.SecurityModuleEvent_GetEventStreamServer) error {
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
		case msg := <-a.events:
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

func (a *APIServer) dequeue(now time.Time, cb func(msg *pendingMsg, retry bool) bool) {
	a.queueLock.Lock()
	defer a.queueLock.Unlock()

	queueSize := len(a.queue)

	a.queue = slicesDeleteUntilFalse(a.queue, func(msg *pendingMsg) bool {
		// apply the delay only if the queue is not full
		if queueSize < a.cfg.EventRetryQueueThreshold && msg.sendAfter.After(now) {
			return false
		}

		if cb(msg, queueSize < a.cfg.EventRetryQueueThreshold) {
			queueSize--
			return true
		}

		msgMaxRetry := msg.getMaxRetry()
		if msg.retry >= msgMaxRetry {
			seclog.Warnf("max retry reached: %d, sending event anyway", msg.retry)

			queueSize--
			return true
		}
		seclog.Warnf("failed to send event for rule `%s`, retry %d/%d, queue size: %d", msg.ruleID, msg.retry, msgMaxRetry, len(a.queue))

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
			if after, ok := strings.CutPrefix(tag, "service:"); ok {
				msg.Service = after
				break
			}
		}
	}
}

func (a *APIServer) updateMsgTrack(msg *api.SecurityEventMessage) {
	if slices.Contains(events.AllSecInfoRuleIDs(), msg.RuleID) {
		msg.Track = string(common.SecInfo)
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

	// in sidecar, append global tags on custom events
	if fargate.IsSidecar() {
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
			a.dequeue(now, func(msg *pendingMsg, isRetryAllowed bool) bool {
				if !isRetryAllowed {
					seclog.Debugf("queue limit reached: %d, sending event anyway", len(a.queue))
				}

				if msg.extTagsCb != nil && isRetryAllowed {
					tags, retryable := msg.extTagsCb()
					if len(tags) == 0 && retryable && msg.retry < msg.getMaxRetry() {
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
				if !msg.isResolved() && isRetryAllowed && msg.retry < msg.getMaxRetry() {
					return false
				}

				if a.containerFilter != nil {
					containerName, imageName, podNamespace := utils.GetContainerFilterTags(msg.tags)
					if a.containerFilter.IsExcluded(nil, containerName, imageName, podNamespace) {
						// similar return value as if we had sent the message
						return true
					}
				}

				data, err := msg.toJSON()
				if err != nil {
					seclog.Errorf("failed to marshal event context: %v", err)
					return true
				}

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
	if a.securityAgentAPIClient != nil {
		seclog.Infof("starting to send events to security agent")

		go a.securityAgentAPIClient.SendEvents(ctx, a.events, func() {
			if prev := a.connEstablished.Swap(true); !prev {
				// should always be non nil
				if a.cwsConsumer != nil {
					a.cwsConsumer.onAPIConnectionEstablished()
				}
			}
		})
		go a.securityAgentAPIClient.SendActivityDumps(ctx, a.activityDumps)
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.start(ctx)
	}()
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
	originalRuleID := rule.Def.ID
	groupRuleID := rule.Def.ID
	if rule.Def.GroupID != "" {
		groupRuleID = rule.Def.GroupID
	}

	backendEvent := events.BackendEvent{
		Title: rule.Def.Description,
		AgentContext: events.AgentContext{
			RuleID:         groupRuleID,
			OriginalRuleID: originalRuleID,
			RuleVersion:    rule.Def.Version,
			Version:        version.AgentVersion,
			OS:             runtime.GOOS,
			Arch:           utils.RuntimeArch(),
			Origin:         a.probe.Origin(),
			KernelVersion:  a.kernelVersion,
			Distribution:   a.distribution,
			PolicyName:     rule.Policy.Name,
			PolicyVersion:  rule.Policy.Version,
		},
	}

	// no retention if there is no ext tags to resolve
	retention := a.retention
	if extTagsCb == nil {
		retention = 0
	}

	// get type tags + container tags if already resolved, see ResolveContainerTags
	eventTags := event.GetTags()

	tags := []string{"rule_id:" + groupRuleID}
	tags = append(tags, "source_rule_id:"+originalRuleID)
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
		// Create SSH session patcher if the event has an SSH user session
		sshSessionPatcher := createSSHSessionPatcher(ev, a.probe)
		timestamp := ev.ResolveEventTime()
		if timestamp.IsZero() {
			timestamp = time.Now()
		}

		msg := &pendingMsg{
			ruleID:            groupRuleID,
			backendEvent:      backendEvent,
			eventSerializer:   serializers.NewEventSerializer(ev, rule, a.probe.GetScrubber()),
			extTagsCb:         extTagsCb,
			service:           service,
			timestamp:         timestamp,
			sendAfter:         time.Now().Add(retention),
			tags:              tags,
			actionReports:     actionReports,
			sshSessionPatcher: sshSessionPatcher,
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

		// for custom events, we can use the current time as timestamp
		timestamp := time.Now()

		m := &api.SecurityEventMessage{
			RuleID:         groupRuleID,
			OriginalRuleID: originalRuleID,
			Data:           data,
			Service:        service,
			Tags:           tags,
			Timestamp:      timestamppb.New(timestamp),
		}
		a.updateCustomEventTags(m)
		a.updateMsgService(m)
		a.updateMsgTrack(m)

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
}

// expireDump updates the count of expired dumps
func (a *APIServer) expireDump(dump *api.ActivityDumpStreamMessage) {
	// update metric
	a.expiredDumps.Inc()
}

// getStats returns a map indexed by ruleIDs that describes the amount of events
// that were expired or rate limited before reaching
func (a *APIServer) getStats() map[string]int64 {
	a.expiredEventsLock.RLock()
	defer a.expiredEventsLock.RUnlock()

	stats := make(map[string]int64, len(a.expiredEvents))
	for ruleID, val := range a.expiredEvents {
		stats[ruleID] = val.Swap(0)
	}
	return stats
}

// SendStats sends statistics
func (a *APIServer) SendStats() error {
	// statistics about the number of dropped events
	for ruleID, val := range a.getStats() {
		tags := []string{"rule_id:" + ruleID}
		if val > 0 {
			if err := a.statsdClient.Count(metrics.MetricEventServerExpired, val, tags, 1.0); err != nil {
				return err
			}
		}
	}

	// telemetry for msg senders
	a.msgSender.SendTelemetry(a.statsdClient)
	a.activityDumpSender.SendTelemetry(a.statsdClient)

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
		return nil, errors.New("failed to get loaded rule set")
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
		RuleSetReportMessage: transform.FromFilterReportToProtoRuleSetReportMessage(report),
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

// Stop stops the API server. The start goroutine must finish before the
// stopper closes pipeline channels, otherwise sends to logChan will panic.
func (a *APIServer) Stop() {
	// Wait for the start goroutine to exit (triggered by context cancellation)
	// before stopping pipeline providers which close the underlying channels.
	a.wg.Wait()
	a.stopper.Stop()
}

// GetStatus returns the status of the module
func (a *APIServer) GetStatus(_ context.Context, _ *api.GetStatusParams) (*api.Status, error) {
	var apiStatus api.Status

	if a.cfg.SendPayloadsFromSystemProbe {
		var endpointsStatus []string
		if senderStatus, ok := a.msgSender.(EndpointsStatusFetcher); ok {
			endpointsStatus = append(endpointsStatus, senderStatus.GetEndpointsStatus()...)
		}
		if dumpSenderStatus, ok := a.activityDumpSender.(EndpointsStatusFetcher); ok {
			endpointsStatus = append(endpointsStatus, dumpSenderStatus.GetEndpointsStatus()...)
		}
		apiStatus.DirectSenderStatus = &api.DirectSenderStatus{
			Endpoints: endpointsStatus,
		}
	}

	if a.selfTester != nil {
		apiStatus.SelfTests = a.selfTester.GetStatus()
	}
	apiStatus.PoliciesStatus = a.policiesStatus

	if err := a.fillStatusPlatform(&apiStatus); err != nil {
		return nil, err
	}

	return &apiStatus, nil
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
			tags = append(tags, env+":"+value)
		}
	}
	return tags
}

// NewAPIServer returns a new gRPC event server
func NewAPIServer(cfg *config.RuntimeSecurityConfig, probe *sprobe.Probe, msgSender MsgSender[api.SecurityEventMessage], client statsd.ClientInterface, selfTester *selftests.SelfTester, compression compression.Component, hostname string) (*APIServer, error) {
	stopper := startstop.NewSerialStopper()
	containerFilter, err := utils.NewContainerFilter()
	if err != nil {
		return nil, err
	}

	as := &APIServer{
		events:          make(chan *api.SecurityEventMessage, cfg.EventServerBurst*3),
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
		containerFilter: containerFilter,
	}

	if !cfg.SendPayloadsFromSystemProbe && cfg.EventGRPCServer == "security-agent" {
		seclog.Infof("setting up security agent api client")

		securityAgentAPIClient, err := NewSecurityAgentAPIClient(cfg)
		if err != nil {
			return nil, err
		}
		as.securityAgentAPIClient = securityAgentAPIClient
	}

	as.collectOSReleaseData()

	if as.msgSender == nil {
		if cfg.SendPayloadsFromSystemProbe {
			msgSender, err := NewDirectEventMsgSender(stopper, compression, hostname)
			if err != nil {
				log.Errorf("failed to setup direct event sender: %v", err)
			} else {
				as.msgSender = msgSender
			}
		}

		if as.msgSender == nil {
			as.msgSender = NewChanMsgSender(as.events)
		}
	}

	if as.activityDumpSender == nil {
		if cfg.SendPayloadsFromSystemProbe {
			adSender, err := NewDirectActivityDumpMsgSender()
			if err != nil {
				log.Errorf("failed to setup direct activity dump sender: %v", err)
			} else {
				as.activityDumpSender = adSender
			}
		}

		if as.activityDumpSender == nil {
			as.activityDumpSender = NewChanMsgSender(as.activityDumps)
		}
	}

	return as, nil
}

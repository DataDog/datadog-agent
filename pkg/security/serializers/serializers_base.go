// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package serializers

import (
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// ContainerContextSerializer serializes a container context to JSON
// easyjson:json
type ContainerContextSerializer struct {
	// Container ID
	ID string `json:"id,omitempty"`
	// Creation time of the container
	CreatedAt *utils.EasyjsonTime `json:"created_at,omitempty"`
}

// MatchedRuleSerializer serializes a rule
// easyjson:json
type MatchedRuleSerializer struct {
	// ID of the rule
	ID string `json:"id,omitempty"`
	// Version of the rule
	Version string `json:"version,omitempty"`
	// Tags of the rule
	Tags []string `json:"tags,omitempty"`
	// Name of the policy that introduced the rule
	PolicyName string `json:"policy_name,omitempty"`
	// Version of the policy that introduced the rule
	PolicyVersion string `json:"policy_version,omitempty"`
}

// EventContextSerializer serializes an event context to JSON
// easyjson:json
type EventContextSerializer struct {
	// Event name
	Name string `json:"name,omitempty"`
	// Event category
	Category string `json:"category,omitempty"`
	// Event outcome
	Outcome string `json:"outcome,omitempty"`
	// True if the event was asynchronous
	Async bool `json:"async,omitempty"`
	// The list of rules that the event matched (only valid in the context of an anomaly)
	MatchedRules []MatchedRuleSerializer `json:"matched_rules,omitempty"`
}

// ProcessContextSerializer serializes a process context to JSON
// easyjson:json
type ProcessContextSerializer struct {
	*ProcessSerializer
	// Parent process
	Parent *ProcessSerializer `json:"parent,omitempty"`
	// Ancestor processes
	Ancestors []*ProcessSerializer `json:"ancestors,omitempty"`
}

// IPPortSerializer is used to serialize an IP and Port context to JSON
// easyjson:json
type IPPortSerializer struct {
	// IP address
	IP string `json:"ip"`
	// Port number
	Port uint16 `json:"port"`
}

// IPPortFamilySerializer is used to serialize an IP, port, and address family context to JSON
// easyjson:json
type IPPortFamilySerializer struct {
	// Address family
	Family string `json:"family"`
	// IP address
	IP string `json:"ip"`
	// Port number
	Port uint16 `json:"port"`
}

// NetworkContextSerializer serializes the network context to JSON
// easyjson:json
type NetworkContextSerializer struct {
	// device is the network device on which the event was captured
	Device *NetworkDeviceSerializer `json:"device,omitempty"`

	// l3_protocol is the layer 3 protocol name
	L3Protocol string `json:"l3_protocol"`
	// l4_protocol is the layer 4 protocol name
	L4Protocol string `json:"l4_protocol"`
	// source is the emitter of the network event
	Source IPPortSerializer `json:"source"`
	// destination is the receiver of the network event
	Destination IPPortSerializer `json:"destination"`
	// size is the size in bytes of the network event
	Size uint32 `json:"size"`
}

// DNSQuestionSerializer serializes a DNS question to JSON
// easyjson:json
type DNSQuestionSerializer struct {
	// class is the class looked up by the DNS question
	Class string `json:"class"`
	// type is a two octet code which specifies the DNS question type
	Type string `json:"type"`
	// name is the queried domain name
	Name string `json:"name"`
	// size is the total DNS request size in bytes
	Size uint16 `json:"size"`
	// count is the total count of questions in the DNS request
	Count uint16 `json:"count"`
}

// DNSEventSerializer serializes a DNS event to JSON
// easyjson:json
type DNSEventSerializer struct {
	// id is the unique identifier of the DNS request
	ID uint16 `json:"id"`
	// question is a DNS question for the DNS request
	Question DNSQuestionSerializer `json:"question"`
}

// DDContextSerializer serializes a span context to JSON
// easyjson:json
type DDContextSerializer struct {
	// Span ID used for APM correlation
	SpanID uint64 `json:"span_id,omitempty"`
	// Trace ID used for APM correlation
	TraceID uint64 `json:"trace_id,omitempty"`
}

// ExitEventSerializer serializes an exit event to JSON
// easyjson:json
type ExitEventSerializer struct {
	// Cause of the process termination (one of EXITED, SIGNALED, COREDUMPED)
	Cause string `json:"cause"`
	// Exit code of the process or number of the signal that caused the process to terminate
	Code uint32 `json:"code"`
}

// SecurityProfileContextSerializer serializes the security profile context in an event
type SecurityProfileContextSerializer struct {
	// Name of the security profile
	Name string `json:"name"`
	// Status defines in which state the security profile was when the event was triggered
	Status string `json:"status"`
	// Version of the profile in use
	Version string `json:"version"`
	// List of tags associated to this profile
	Tags []string `json:"tags"`
}

// BaseEventSerializer serializes an event to JSON
// easyjson:json
type BaseEventSerializer struct {
	EventContextSerializer `json:"evt,omitempty"`
	Date                   utils.EasyjsonTime `json:"date,omitempty"`

	*FileEventSerializer              `json:"file,omitempty"`
	*DNSEventSerializer               `json:"dns,omitempty"`
	*NetworkContextSerializer         `json:"network,omitempty"`
	*ExitEventSerializer              `json:"exit,omitempty"`
	*ProcessContextSerializer         `json:"process,omitempty"`
	*DDContextSerializer              `json:"dd,omitempty"`
	*ContainerContextSerializer       `json:"container,omitempty"`
	*SecurityProfileContextSerializer `json:"security_profile,omitempty"`
}

func newSecurityProfileContextSerializer(e *model.SecurityProfileContext) *SecurityProfileContextSerializer {
	tags := make([]string, len(e.Tags))
	copy(tags, e.Tags)
	return &SecurityProfileContextSerializer{
		Name:    e.Name,
		Version: e.Version,
		Status:  e.Status.String(),
		Tags:    tags,
	}
}

func newMatchedRulesSerializer(r *model.MatchedRule) MatchedRuleSerializer {
	mrs := MatchedRuleSerializer{
		ID:            r.RuleID,
		Version:       r.RuleVersion,
		PolicyName:    r.PolicyName,
		PolicyVersion: r.PolicyVersion,
		Tags:          make([]string, 0, len(r.RuleTags)),
	}

	for tagName, tagValue := range r.RuleTags {
		mrs.Tags = append(mrs.Tags, tagName+":"+tagValue)
	}
	return mrs
}

func newDDContextSerializer(e *model.Event) *DDContextSerializer {
	s := &DDContextSerializer{
		SpanID:  e.SpanContext.SpanID,
		TraceID: e.SpanContext.TraceID,
	}
	if s.SpanID != 0 || s.TraceID != 0 {
		return s
	}

	ctx := eval.NewContext(e)
	it := &model.ProcessAncestorsIterator{}
	ptr := it.Front(ctx)

	for ptr != nil {
		pce := (*model.ProcessCacheEntry)(ptr)

		if pce.SpanID != 0 || pce.TraceID != 0 {
			s.SpanID = pce.SpanID
			s.TraceID = pce.TraceID
			break
		}

		ptr = it.Next()
	}

	return s
}

// nolint: deadcode, unused
func newDNSEventSerializer(d *model.DNSEvent) *DNSEventSerializer {
	return &DNSEventSerializer{
		ID: d.ID,
		Question: DNSQuestionSerializer{
			Class: model.QClass(d.Class).String(),
			Type:  model.QType(d.Type).String(),
			Name:  d.Name,
			Size:  d.Size,
			Count: d.Count,
		},
	}
}

// nolint: deadcode, unused
func newIPPortSerializer(c *model.IPPortContext) IPPortSerializer {
	return IPPortSerializer{
		IP:   c.IPNet.IP.String(),
		Port: c.Port,
	}
}

// nolint: deadcode, unused
func newIPPortFamilySerializer(c *model.IPPortContext, family string) IPPortFamilySerializer {
	return IPPortFamilySerializer{
		IP:     c.IPNet.IP.String(),
		Port:   c.Port,
		Family: family,
	}
}

// nolint: deadcode, unused
func newNetworkContextSerializer(e *model.Event) *NetworkContextSerializer {
	return &NetworkContextSerializer{
		Device:      newNetworkDeviceSerializer(e),
		L3Protocol:  model.L3Protocol(e.NetworkContext.L3Protocol).String(),
		L4Protocol:  model.L4Protocol(e.NetworkContext.L4Protocol).String(),
		Source:      newIPPortSerializer(&e.NetworkContext.Source),
		Destination: newIPPortSerializer(&e.NetworkContext.Destination),
		Size:        e.NetworkContext.Size,
	}
}

func newExitEventSerializer(e *model.Event) *ExitEventSerializer {
	return &ExitEventSerializer{
		Cause: model.ExitCause(e.Exit.Cause).String(),
		Code:  e.Exit.Code,
	}
}

// NewBaseEventSerializer creates a new event serializer based on the event type
func NewBaseEventSerializer(event *model.Event, resolvers *resolvers.Resolvers) *BaseEventSerializer {
	pc := event.ProcessContext

	eventType := model.EventType(event.Type)

	s := &BaseEventSerializer{
		EventContextSerializer: EventContextSerializer{
			Name: eventType.String(),
		},
		ProcessContextSerializer: newProcessContextSerializer(pc, event, resolvers),
		DDContextSerializer:      newDDContextSerializer(event),
		Date:                     utils.NewEasyjsonTime(event.FieldHandlers.ResolveEventTime(event)),
	}

	if event.IsAnomalyDetectionEvent() && len(event.Rules) > 0 {
		s.EventContextSerializer.MatchedRules = make([]MatchedRuleSerializer, 0, len(event.Rules))
		for _, r := range event.Rules {
			s.EventContextSerializer.MatchedRules = append(s.EventContextSerializer.MatchedRules, newMatchedRulesSerializer(r))
		}
	}

	s.Category = model.GetEventTypeCategory(eventType.String())
	if s.Category == model.NetworkCategory {
		s.NetworkContextSerializer = newNetworkContextSerializer(event)
	}

	if event.SecurityProfileContext.Name != "" {
		s.SecurityProfileContextSerializer = newSecurityProfileContextSerializer(&event.SecurityProfileContext)
	}

	switch eventType {
	case model.ExitEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.ProcessContext.Process.FileEvent, event),
		}
		s.ExitEventSerializer = newExitEventSerializer(event)
		s.EventContextSerializer.Outcome = serializeOutcome(0)
	case model.ExecEventType:
		s.FileEventSerializer = &FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.ProcessContext.Process.FileEvent, event),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(0)
	}

	return s
}

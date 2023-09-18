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
	smodel "github.com/DataDog/datadog-agent/pkg/security/serializers/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func newSecurityProfileContextSerializer(e *model.SecurityProfileContext) *smodel.SecurityProfileContextSerializer {
	tags := make([]string, len(e.Tags))
	copy(tags, e.Tags)
	return &smodel.SecurityProfileContextSerializer{
		Name:    e.Name,
		Version: e.Version,
		Status:  e.Status.String(),
		Tags:    tags,
	}
}

func newMatchedRulesSerializer(r *model.MatchedRule) smodel.MatchedRuleSerializer {
	mrs := smodel.MatchedRuleSerializer{
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

func newDDContextSerializer(e *model.Event) *smodel.DDContextSerializer {
	s := &smodel.DDContextSerializer{
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
func newDNSEventSerializer(d *model.DNSEvent) *smodel.DNSEventSerializer {
	return &smodel.DNSEventSerializer{
		ID: d.ID,
		Question: smodel.DNSQuestionSerializer{
			Class: model.QClass(d.Class).String(),
			Type:  model.QType(d.Type).String(),
			Name:  d.Name,
			Size:  d.Size,
			Count: d.Count,
		},
	}
}

// nolint: deadcode, unused
func newIPPortSerializer(c *model.IPPortContext) smodel.IPPortSerializer {
	return smodel.IPPortSerializer{
		IP:   c.IPNet.IP.String(),
		Port: c.Port,
	}
}

// nolint: deadcode, unused
func newIPPortFamilySerializer(c *model.IPPortContext, family string) smodel.IPPortFamilySerializer {
	return smodel.IPPortFamilySerializer{
		IP:     c.IPNet.IP.String(),
		Port:   c.Port,
		Family: family,
	}
}

// nolint: deadcode, unused
func newNetworkContextSerializer(e *model.Event) *smodel.NetworkContextSerializer {
	return &smodel.NetworkContextSerializer{
		Device:      newNetworkDeviceSerializer(e),
		L3Protocol:  model.L3Protocol(e.NetworkContext.L3Protocol).String(),
		L4Protocol:  model.L4Protocol(e.NetworkContext.L4Protocol).String(),
		Source:      newIPPortSerializer(&e.NetworkContext.Source),
		Destination: newIPPortSerializer(&e.NetworkContext.Destination),
		Size:        e.NetworkContext.Size,
	}
}

func newExitEventSerializer(e *model.Event) *smodel.ExitEventSerializer {
	return &smodel.ExitEventSerializer{
		Cause: model.ExitCause(e.Exit.Cause).String(),
		Code:  e.Exit.Code,
	}
}

// NewBaseEventSerializer creates a new event serializer based on the event type
func NewBaseEventSerializer(event *model.Event, resolvers *resolvers.Resolvers) *smodel.BaseEventSerializer {
	pc := event.ProcessContext

	eventType := model.EventType(event.Type)

	s := &smodel.BaseEventSerializer{
		EventContextSerializer: smodel.EventContextSerializer{
			Name: eventType.String(),
		},
		ProcessContextSerializer: newProcessContextSerializer(pc, event, resolvers),
		DDContextSerializer:      newDDContextSerializer(event),
		Date:                     utils.NewEasyjsonTime(event.FieldHandlers.ResolveEventTime(event)),
	}

	if event.IsAnomalyDetectionEvent() && len(event.Rules) > 0 {
		s.EventContextSerializer.MatchedRules = make([]smodel.MatchedRuleSerializer, 0, len(event.Rules))
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
		s.FileEventSerializer = &smodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.ProcessContext.Process.FileEvent, event),
		}
		s.ExitEventSerializer = newExitEventSerializer(event)
		s.EventContextSerializer.Outcome = serializeOutcome(0)
	case model.ExecEventType:
		s.FileEventSerializer = &smodel.FileEventSerializer{
			FileSerializer: *newFileSerializer(&event.ProcessContext.Process.FileEvent, event),
		}
		s.EventContextSerializer.Outcome = serializeOutcome(0)
	}

	return s
}

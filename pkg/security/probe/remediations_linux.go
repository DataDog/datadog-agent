//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=readonly -no_std_marshalers -build_tags linux $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	json "encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes/rawpacket"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// remediationAction tracks the state of an applied remediation
type remediationAction struct {
	actionType         uint8
	scope              string
	triggered          bool
	containerID        string
	containerCreatedAt uint64
	policy             string
	isolationReApplied bool
	isolationNew       bool

	ruleTags RuleTags
}

const (
	// remediationActionTypeKill
	remediationActionTypeKill uint8 = iota

	// remediationActionTypeNetworkIsolation
	remediationActionTypeNetworkIsolation
)

type RuleTags map[string]string

// RemediationContainerContext represents the container context for remediation events
type RemediationContainerContext struct {
	CreatedAt uint64 `json:"created_at,omitempty"`
	ID        string `json:"id,omitempty"`
}

// RemediationAgentContext represents the agent context for remediation events
type RemediationAgentContext struct {
	RuleID        string `json:"rule_id"`
	OS            string `json:"os"`
	KernelVersion string `json:"kernel_version"`
	Origin        string `json:"origin"`
	Arch          string `json:"arch"`
	Distribution  string `json:"distribution"`
	Version       string `json:"version"`
}

// RemediationEvent defines a custom remediation event
type RemediationEvent struct {
	Date              string                       `json:"date"`
	Container         *RemediationContainerContext `json:"container,omitempty"`
	Agent             RemediationAgentContext      `json:"agent"`
	EventType         string                       `json:"event_type"`
	Service           string                       `json:"service"`
	Scope             string                       `json:"scope"`
	RemediationAction string                       `json:"remediation_action"`
	Status            string                       `json:"status"`
	Timestamp         int64                        `json:"timestamp"`
	RuleTags          RuleTags                     `json:"rule_tags,omitempty"`
}

// ToJSON marshal the kill action event
func (k *RemediationEvent) ToJSON() ([]byte, error) {
	return json.Marshal(k)
}

func getAgentEventID(rule *rules.Rule) string {
	for _, tag := range rule.Tags {
		if strings.HasPrefix(tag, "agent_event_id:") {
			return tag[len("agent_event_id:"):]
		}
	}
	return ""
}

func getRemediationTagBool(rule *rules.Rule) bool {
	for _, tag := range rule.Tags {
		if strings.HasPrefix(tag, "remediation_rule:") {
			if tag[len("remediation_rule:"):] == "true" {
				return true
			}
		}
	}
	return false
}

// NewRemediationEvent creates a new network remediation event from the latest action report
// activeRemediationsLock must be held
func NewRemediationEvent(p *EBPFProbe, agentID string, status string, remediationAction string) *RemediationEvent {
	remediation := p.activeRemediations[agentID]
	containerContext := &RemediationContainerContext{
		CreatedAt: remediation.containerCreatedAt,
		ID:        remediation.containerID,
	}

	now := time.Now()

	// Build agent context
	kernelVersion := p.GetKernelVersion()
	distribution := fmt.Sprintf("%s - %s", kernelVersion.OsRelease["ID"], kernelVersion.OsRelease["VERSION_ID"])

	agentContext := RemediationAgentContext{
		RuleID:        events.RemediationStatusRuleID,
		OS:            "linux",
		KernelVersion: kernelVersion.Code.String(),
		Origin:        p.probe.Origin(),
		Arch:          utils.RuntimeArch(),
		Distribution:  distribution,
		Version:       version.AgentVersion,
	}

	return &RemediationEvent{
		Date:              now.Format(time.RFC3339Nano),
		Container:         containerContext,
		Agent:             agentContext,
		EventType:         "remediation_status",
		Service:           "runtime-security-agent",
		Scope:             remediation.scope,
		Status:            status,
		Timestamp:         now.UnixMilli(),
		RemediationAction: remediationAction,
		RuleTags:          remediation.ruleTags,
	}
}

func getTagsFromRule(rule *rules.Rule) RuleTags {
	ruleTags := make(RuleTags)

	for _, tag := range rule.Tags {
		// Extract key:value from "key:value"
		if before, after, ok := strings.Cut(tag, ":"); ok {
			key := before
			value := after
			ruleTags[key] = value
		}
	}
	return ruleTags
}

func (p *EBPFProbe) handleRemediationStatus(rs *rules.RuleSet) {
	p.activeRemediationsLock.Lock()
	defer p.activeRemediationsLock.Unlock()
	// First, remove all the previous kill actions because only network isolation actions can be persistent
	for key, state := range p.activeRemediations {
		if state.actionType == remediationActionTypeKill {
			delete(p.activeRemediations, key)
		}
	}

	// When a new ruleset is loaded, reset all remediation flags
	for _, state := range p.activeRemediations {
		state.triggered = false
		state.isolationReApplied = false
		state.isolationNew = false
	}

	for _, rule := range rs.GetRules() {
		for _, action := range rule.Actions {
			var actionType uint8
			if action.Def.NetworkFilter == nil && action.Def.Kill == nil {
				// This is not a kill or network isolation action
				continue
			}
			if !getRemediationTagBool(rule) {
				// This is not a remediation action
				continue
			}
			agentEventID := getAgentEventID(rule)

			if action.Def.NetworkFilter != nil {
				actionType = remediationActionTypeNetworkIsolation
				// If isolation is new, create a new entry in the map
				if _, exists := p.activeRemediations[agentEventID]; !exists {
					p.activeRemediations[agentEventID] = &remediationAction{
						actionType:   actionType,
						triggered:    false,
						isolationNew: true,
						scope:        action.Def.NetworkFilter.Scope,
						ruleTags:     getTagsFromRule(rule),
					}
				} else {
					// Otherwise, update the entry
					p.activeRemediations[agentEventID].isolationReApplied = true
				}
			} else if action.Def.Kill != nil {
				actionType = remediationActionTypeKill
				// Create a new entry for kill actions
				p.activeRemediations[agentEventID] = &remediationAction{
					actionType: actionType,
					triggered:  false,
					scope:      action.Def.Kill.Scope,
					ruleTags:   getTagsFromRule(rule),
				}
			}
		}
	}
	// After all the rule are laoded, check if some isolation actions were removed
	for agentEventID, state := range p.activeRemediations {
		if state.actionType == remediationActionTypeNetworkIsolation && !state.isolationReApplied && !state.isolationNew {
			networkFilterEvent := NewRemediationEvent(p, agentEventID, "removed", "cancel_network_isolation")
			p.SendCustomRemediationEvent(networkFilterEvent)
			delete(p.activeRemediations, agentEventID)
		}
	}
}

func (p *EBPFProbe) handleKillRemediaitonAction(rule *rules.Rule, ev *model.Event) {
	agentEventID := getAgentEventID(rule)
	if agentEventID != "" && getRemediationTagBool(rule) {
		if killReport, ok := ev.ActionReports[len(ev.ActionReports)-1].(*KillActionReport); ok {
			p.activeRemediationsLock.Lock()
			defer p.activeRemediationsLock.Unlock()
			remediation, found := p.activeRemediations[agentEventID]
			// Send network filter action only for remediation events, so when an agent_event_id is set

			// Record that the kill action was triggered
			if found {
				remediation.triggered = true
				remediation.containerID = string(ev.ProcessContext.Process.ContainerContext.ContainerID)
				remediation.containerCreatedAt = ev.ProcessContext.Process.ContainerContext.CreatedAt
				remediation.policy = ""
				remediation.ruleTags = getTagsFromRule(rule)

			}
			// Get kill status
			killReport.RLock()
			status := string(killReport.Status)
			killReport.RUnlock()
			// Send custom event
			killActionEvent := NewRemediationEvent(p, agentEventID, status, "kill")
			p.SendCustomRemediationEvent(killActionEvent)
		}
	}

}

func (p *EBPFProbe) handleNetworkRemediaitonAction(rule *rules.Rule, ev *model.Event, policy rawpacket.Policy, status string) {
	agentEventID := getAgentEventID(rule)
	if agentEventID != "" && getRemediationTagBool(rule) {
		// Send network filter action only for remediation events, so when an agent_event_id is set
		p.activeRemediationsLock.Lock()
		defer p.activeRemediationsLock.Unlock()

		remediation, found := p.activeRemediations[agentEventID]

		// store that the rule triggered
		if found {
			// New isolation: apply it and send the event
			remediation.triggered = true
			if remediation.isolationReApplied {
				// We already re-applied the isolation, no need to send the event
				return
			}
			remediation.containerID = string(ev.ProcessContext.Process.ContainerContext.ContainerID)
			remediation.containerCreatedAt = ev.ProcessContext.Process.ContainerContext.CreatedAt
			remediation.policy = policy.String()
			networkFilterEvent := NewRemediationEvent(p, agentEventID, status, "network_isolation")
			p.SendCustomRemediationEvent(networkFilterEvent)
		}
	}

}

func (p *EBPFProbe) SendCustomRemediationEvent(re *RemediationEvent) {
	customRule := events.NewCustomRule(events.RemediationStatusRuleID, events.RemediationStatusRuleDesc)
	customEvent := events.NewCustomEvent(model.CustomEventType, re)
	p.probe.DispatchCustomEvent(customRule, customEvent)

}
func (p *EBPFProbe) handleRemediationNotTriggered() {
	p.activeRemediationsLock.Lock()
	for agentEventID, state := range p.activeRemediations {
		var remediationAction string
		if !state.triggered {
			if state.actionType == remediationActionTypeNetworkIsolation {
				remediationAction = "network_isolation"
			} else if state.actionType == remediationActionTypeKill {
				remediationAction = "kill"
			}
			re := NewRemediationEvent(p, agentEventID, "not_triggered", remediationAction)
			p.SendCustomRemediationEvent(re)
		}
	}
	p.activeRemediationsLock.Unlock()

}

//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=readonly -no_std_marshalers -build_tags linux $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// HashTriggerTimeout hash triggered because of a timeout
	HashTriggerTimeout = "timeout"
	// HashTriggerProcessExit hash triggered on process exit
	HashTriggerProcessExit = "process_exit"
)

// HashActionReport defines a hash action reports
// easyjson:json
type HashActionReport struct {
	sync.RWMutex

	Type    string `json:"type"`
	Path    string `json:"path"`
	State   string `json:"state"`
	Trigger string `json:"trigger"`

	// internal
	resolved  bool
	rule      *rules.Rule
	pid       uint32
	seenAt    time.Time
	fileEvent model.FileEvent
	crtID     containerutils.ContainerID
	eventType model.EventType
}

// IsResolved return if the action is resolved
func (k *HashActionReport) IsResolved() error {
	k.RLock()
	defer k.RUnlock()

	if k.resolved {
		return nil
	}

	return fmt.Errorf("hash action current state: %+v", k)
}

// ToJSON marshal the action
func (k *HashActionReport) ToJSON() ([]byte, error) {
	k.Lock()
	defer k.Unlock()

	k.Type = rules.HashAction
	k.Path = k.fileEvent.PathnameStr
	k.State = k.fileEvent.HashState.String()

	data, err := utils.MarshalEasyJSON(k)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// IsMatchingRule returns true if this action report is targeted at the given rule ID
func (k *HashActionReport) IsMatchingRule(ruleID eval.RuleID) bool {
	k.RLock()
	defer k.RUnlock()

	return k.rule.ID == ruleID
}

// PatchEvent implements the EventSerializerPatcher interface
func (k *HashActionReport) PatchEvent(ev *serializers.EventSerializer) {
	if ev.FileEventSerializer == nil {
		return
	}

	ev.FileEventSerializer.HashState = k.fileEvent.HashState.String()
	ev.FileEventSerializer.Hashes = k.fileEvent.Hashes
}

// RawPacketActionReport defines a raw packet action reports
// easyjson:json
type RawPacketActionReport struct {
	sync.RWMutex

	Filter string `json:"filter"`
	Policy string `json:"policy"`

	// internal
	rule *rules.Rule
}

// IsResolved return if the action is resolved
func (k *RawPacketActionReport) IsResolved() error {
	return nil
}

// ToJSON marshal the action
func (k *RawPacketActionReport) ToJSON() ([]byte, error) {
	k.Lock()
	defer k.Unlock()

	data, err := utils.MarshalEasyJSON(k)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// IsMatchingRule returns true if this action report is targeted at the given rule ID
func (k *RawPacketActionReport) IsMatchingRule(ruleID eval.RuleID) bool {
	k.RLock()
	defer k.RUnlock()

	return k.rule.ID == ruleID
}

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

// RemediationEvent defines a kill action event
type RemediationEvent struct {
	Date         string                       `json:"date"`
	Container    *RemediationContainerContext `json:"container,omitempty"`
	Agent        RemediationAgentContext      `json:"agent"`
	EventType    string                       `json:"event_type"`
	Hostname     string                       `json:"hostname"`
	Service      string                       `json:"service"`
	Title        string                       `json:"title"`
	Status       string                       `json:"status"`
	AgentEventID string                       `json:"agent_event_id"`
	Timestamp    int64                        `json:"timestamp"`
}

// ToJSON marshal the kill action event
func (k *RemediationEvent) ToJSON() ([]byte, error) {
	return json.Marshal(k)
}

// NewKillRemediationEvent creates a new kill action event from the latest action report
func NewKillRemediationEvent(p *EBPFProbe, rule *rules.Rule, report *KillActionReport, event *model.Event, hostname string) *RemediationEvent {
	report.RLock()
	defer report.RUnlock()

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

	// Build container context if present
	var containerContext *RemediationContainerContext
	process := &event.ProcessContext.Process
	if containerID := string(process.ContainerContext.ContainerID); containerID != "" {
		containerContext = &RemediationContainerContext{
			CreatedAt: process.ContainerContext.CreatedAt,
			ID:        containerID,
		}
	}
	// get agent_event_id in tags
	agentEventID := getAgentEventID(rule)
	return &RemediationEvent{
		Date:         now.Format(time.RFC3339Nano),
		Container:    containerContext,
		Agent:        agentContext,
		EventType:    "remediation_status",
		Hostname:     hostname,
		Service:      "runtime-security-agent",
		Title:        "Process killed",
		Status:       string(report.Status),
		Timestamp:    now.UnixMilli(),
		AgentEventID: agentEventID,
	}
}
func getAgentEventID(rule *rules.Rule) string {
	for _, tag := range rule.Tags {
		if strings.HasPrefix(tag, "agent_event_id:") {
			return tag[len("agent_event_id:"):]
		}
	}
	return ""
}

// NewnetworkRemediationEvent creates a new network remediation event from the latest action report
func NewnetworkRemediationEvent(p *EBPFProbe, rule *rules.Rule, report *RawPacketActionReport, event *model.Event, hostname string) *RemediationEvent {
	report.RLock()
	defer report.RUnlock()

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

	// Build container context if present
	var containerContext *RemediationContainerContext
	process := &event.ProcessContext.Process
	if containerID := string(process.ContainerContext.ContainerID); containerID != "" {
		containerContext = &RemediationContainerContext{
			CreatedAt: process.ContainerContext.CreatedAt,
			ID:        containerID,
		}
	}
	// Build the title with network action details
	title := fmt.Sprintf("Network traffic filtered - Policy: %s", report.Policy)

	agentEventID := getAgentEventID(rule)
	return &RemediationEvent{
		Date:         now.Format(time.RFC3339Nano),
		Container:    containerContext,
		Agent:        agentContext,
		EventType:    "remediation_status",
		Hostname:     hostname,
		Service:      "runtime-security-agent",
		Title:        title,
		Status:       "applied",
		Timestamp:    now.UnixMilli(),
		AgentEventID: agentEventID,
	}
}

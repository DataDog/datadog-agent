// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build !linux && !windows

package probe

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// PlatformProbe represents the no-op platform probe on unsupported platforms
type PlatformProbe struct {
}

// Probe represents the runtime security probe
type Probe struct {
	Config *config.Config
	Opts   Opts
}

// Origin returns origin
func (p *Probe) Origin() string {
	return ""
}

// AddEventHandler set the probe event handler
func (p *Probe) AddEventHandler(_ EventHandler) error {
	return nil
}

// AddCustomEventHandler set the probe event handler
func (p *Probe) AddCustomEventHandler(_ model.EventType, _ CustomEventHandler) error {
	return nil
}

// NewRuleSet returns a new ruleset
func (p *Probe) NewRuleSet(_ map[eval.EventType]bool) *rules.RuleSet {
	return nil
}

// ApplyRuleSet setup the probes for the provided set of rules and returns the policy report.
func (p *Probe) ApplyRuleSet(_ *rules.RuleSet) (*kfilters.ApplyRuleSetReport, error) {
	return nil, nil
}

// OnNewRuleSetLoaded resets statistics and states once a new rule set is loaded
func (p *Probe) OnNewRuleSetLoaded(_ *rules.RuleSet) {
}

// OnNewDiscarder is called when a new discarder is found. We currently don't generate discarders on Windows.
func (p *Probe) OnNewDiscarder(_ *rules.RuleSet, _ *model.Event, _ eval.Field, _ eval.EventType) {
}

// GetService returns the service name from the process tree
func (p *Probe) GetService(_ *model.Event) string {
	return ""
}

// GetEventTags returns the event tags
func (p *Probe) GetEventTags(_ string) []string {
	return nil
}

// IsNetworkEnabled returns whether network is enabled
func (p *Probe) IsNetworkEnabled() bool {
	return p.Config.Probe.NetworkEnabled
}

// IsNetworkRawPacketEnabled returns whether network raw packet is enabled
func (p *Probe) IsNetworkRawPacketEnabled() bool {
	return p.IsNetworkEnabled() && p.Config.Probe.NetworkRawPacketEnabled
}

// IsActivityDumpEnabled returns whether activity dump is enabled
func (p *Probe) IsActivityDumpEnabled() bool {
	return p.Config.RuntimeSecurity.ActivityDumpEnabled
}

// StatsPollingInterval returns polling interval duration
func (p *Probe) StatsPollingInterval() time.Duration {
	return p.Config.Probe.StatsPollingInterval
}

// FlushDiscarders invalidates all the discarders
func (p *Probe) FlushDiscarders() error {
	return nil
}

// RefreshUserCache refreshes the user cache
func (p *Probe) RefreshUserCache(_ string) error {
	return nil
}

// HandleActions executes the actions of a triggered rule
func (p *Probe) HandleActions(_ *rules.Rule, _ eval.Event) {}

// EnableEnforcement sets the enforcement mode
func (p *Probe) EnableEnforcement(_ bool) {}

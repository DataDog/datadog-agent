// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !linux_bpf

// Package probe holds probe related files
package probe

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

type EBPFProbe struct{}

func (ep *EBPFProbe) Init() error {
	return nil
}
func (ep *EBPFProbe) Start() error {
	return nil
}
func (ep *EBPFProbe) Stop() {}
func (ep *EBPFProbe) SendStats() error {
	return nil
}
func (ep *EBPFProbe) Snapshot() error {
	return nil
}
func (ep *EBPFProbe) Walk(_ func(_ *model.ProcessCacheEntry)) {}
func (ep *EBPFProbe) Close() error {
	return nil
}
func (ep *EBPFProbe) NewModel() *model.Model {
	return nil
}
func (ep *EBPFProbe) DumpDiscarders() (string, error) {
	return "", nil
}
func (ep *EBPFProbe) FlushDiscarders() error {
	return nil
}
func (ep *EBPFProbe) ApplyRuleSet(_ *rules.RuleSet) (*kfilters.ApplyRuleSetReport, error) {
	return nil, nil
}
func (ep *EBPFProbe) OnNewRuleSetLoaded(_ *rules.RuleSet) {

}
func (ep *EBPFProbe) OnNewDiscarder(_ *rules.RuleSet, _ *model.Event, _ eval.Field, _ eval.EventType) {
}
func (ep *EBPFProbe) HandleActions(_ *eval.Context, _ *rules.Rule) {}
func (ep *EBPFProbe) NewEvent() *model.Event                       { return nil }
func (ep *EBPFProbe) GetFieldHandlers() model.FieldHandlers {
	return nil
}
func (ep *EBPFProbe) DumpProcessCache(_ bool) (string, error) {
	return "", nil
}
func (ep *EBPFProbe) AddDiscarderPushedCallback(_ DiscarderPushedCallback) {}
func (ep *EBPFProbe) GetEventTags(_ containerutils.ContainerID) []string   { return nil }
func (ep *EBPFProbe) EnableEnforcement(bool)                               {}

// NewEBPFProbe instantiates a new runtime security agent probe
func NewEBPFProbe(probe *Probe, config *config.Config, opts Opts) (*EBPFProbe, error) {
	return nil, errors.New("ebpf not supported")
}

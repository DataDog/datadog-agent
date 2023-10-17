// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ebpfless

// Package probe holds probe related files
package probe

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/safchain/rstrace/pkg/rstrace"
	"google.golang.org/grpc"
)

type PlatformProbe struct {
	Manager *manager.Manager

	// internals
	rstrace.UnimplementedSyscallStreamServer

	kernelVersion *kernel.Version
}

func (p *Probe) SendSyscall(ctx context.Context, syscall *rstrace.Syscall) (*rstrace.Response, error) {
	ev := p.zeroEvent()

	switch syscall.Type {
	case rstrace.ExecSyscallType:
		entry := p.resolvers.AddExecEntry(syscall.PID, syscall.Exec.Filename, syscall.Exec.Args, syscall.Exec.Envs)
		event.Exec.Process = &entry.Process
	case rstrace.ForkSyscallType:
		p.resolvers.AddForkEntry(syscall.PID, syscall.Fork.PPID)
	case rstrace.OpenSyscallType:
		ev.Type = uint32(model.FileOpenEventType)
		ev.OpenFile.SetPathnameStr(syscall.Open.Filename)
	default:
		return &rstrace.Response{}, nil
	}

	// use ProcessCacheEntry process context as process context
	ev.ProcessCacheEntry = p.resolvers.ProcessResolver.Resolve(syscall.PID)
	ev.ProcessContext = &pce.ProcessContext

	p.DispatchEvent(ev)

	return &rstrace.Response{}, nil
}

func (p *Probe) Setup() error {
	return nil
}

func (p *Probe) Init() error {
	p.startTime = time.Now()

	return nil
}

func (p *Probe) Snapshot() error {
	return nil
}

func (p *Probe) Stop() {}

func (p *Probe) Close() error {
	return nil
}

func (p *Probe) sendEventToWildcardHandlers(event *model.Event) {
	for _, handler := range p.fullAccessEventHandlers[model.UnknownEventType] {
		handler.HandleEvent(event)
	}
}

func (p *Probe) sendEventToSpecificEventTypeHandlers(event *model.Event) {
	for _, handler := range p.eventHandlers[event.GetEventType()] {
		handler.HandleEvent(handler.Copy(event))
	}
}

// DispatchEvent sends an event to the probe event handler
func (p *Probe) DispatchEvent(event *model.Event) {

	// send event to wildcard handlers, like the CWS rule engine, first
	p.sendEventToWildcardHandlers(event)

	// send event to specific event handlers, like the event monitor consumers, subsequently
	p.sendEventToSpecificEventTypeHandlers(event)
}

func (p *Probe) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", 7878))
	if err != nil {
		return err
	}
	var opts []grpc.ServerOption

	grpcServer := grpc.NewServer(opts...)
	rstrace.RegisterSyscallStreamServer(grpcServer, p)

	go grpcServer.Serve(lis)

	return nil
}

func (p *Probe) detectKernelVersion() error {
	kernelVersion, err := kernel.NewKernelVersion()
	if err != nil {
		return fmt.Errorf("unable to detect the kernel version: %w", err)
	}
	p.kernelVersion = kernelVersion
	return nil
}

// SendStats sends statistics about the probe to Datadog
func (p *Probe) SendStats() error {
	return nil
}

// GetDebugStats returns the debug stats
func (p *Probe) GetDebugStats() map[string]interface{} {
	debug := map[string]interface{}{
		"start_time": p.startTime.String(),
	}
	return debug
}

// OnNewDiscarder is called when a new discarder is found. We currently don't generate discarders on Windows.
func (p *Probe) OnNewDiscarder(rs *rules.RuleSet, ev *model.Event, field eval.Field, eventType eval.EventType) {
}

// ApplyRuleSet setup the probes for the provided set of rules and returns the policy report.
func (p *Probe) ApplyRuleSet(rs *rules.RuleSet) (*kfilters.ApplyRuleSetReport, error) {
	return kfilters.NewApplyRuleSetReport(p.Config.Probe, rs)
}

// FlushDiscarders invalidates all the discarders
func (p *Probe) FlushDiscarders() error {
	return nil
}

// RefreshUserCache refreshes the user cache
func (p *Probe) RefreshUserCache(containerID string) error {
	return nil
}

func NewProbe(config *config.Config, opts Opts) (*Probe, error) {
	fmt.Printf("---- EBPFLESS!!!!!!!!!\n")

	opts.normalize()

	p := &Probe{
		Config: config,
	}

	if err := p.detectKernelVersion(); err != nil {
		// we need the kernel version to start, fail if we can't get it
		return nil, err
	}

	fmt.Printf("+++++ EBPFLESS!!!!!!!!!\n")

	return p, nil
}

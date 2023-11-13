// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ebpfless

// Package probe holds probe related files
package probe

import (
	"context"
	"net"
	"path/filepath"
	"time"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
)

type PlatformProbe struct {
	// internals
	ebpfless.UnimplementedSyscallMsgStreamServer
	server *grpc.Server
}

func (p *Probe) SendSyscallMsg(ctx context.Context, syscallMsg *ebpfless.SyscallMsg) (*ebpfless.Response, error) {
	event := p.zeroEvent()

	switch syscallMsg.Type {
	case ebpfless.SyscallType_Exec:
		event.Type = uint32(model.ExecEventType)
		entry := p.resolvers.ProcessResolver.AddExecEntry(syscallMsg.PID, syscallMsg.Exec.Filename, syscallMsg.Exec.Args, syscallMsg.Exec.Envs)
		event.Exec.Process = &entry.Process
	case ebpfless.SyscallType_Fork:
		event.Type = uint32(model.ForkEventType)
		p.resolvers.ProcessResolver.AddForkEntry(syscallMsg.PID, syscallMsg.Fork.PPID)
	case ebpfless.SyscallType_Open:
		event.Type = uint32(model.FileOpenEventType)
		event.Open.File.PathnameStr = syscallMsg.Open.Filename
		event.Open.File.BasenameStr = filepath.Base(syscallMsg.Open.Filename)
		event.Open.Flags = syscallMsg.Open.Flags
		event.Open.Mode = syscallMsg.Open.Mode
	default:
		return &ebpfless.Response{}, nil
	}

	// container context
	event.ContainerContext.ID = syscallMsg.ContainerContext.ID
	event.ContainerContext.CreatedAt = syscallMsg.ContainerContext.CreatedAt
	event.ContainerContext.Tags = []string{
		"image_name:" + syscallMsg.ContainerContext.Name,
		"image_tag:" + syscallMsg.ContainerContext.Tag,
	}

	// use ProcessCacheEntry process context as process context
	event.ProcessCacheEntry = p.resolvers.ProcessResolver.Resolve(syscallMsg.PID)
	if event.ProcessCacheEntry == nil {
		event.ProcessCacheEntry = model.NewPlaceholderProcessCacheEntry(syscallMsg.PID)
	}
	event.ProcessContext = &event.ProcessCacheEntry.ProcessContext

	p.DispatchEvent(event)

	return &ebpfless.Response{}, nil
}

func (p *Probe) Setup() error {
	return nil
}

func (p *Probe) Init() error {
	p.startTime = time.Now()

	if err := p.resolvers.Start(p.ctx); err != nil {
		return err
	}

	return nil
}

func (p *Probe) Snapshot() error {
	return nil
}

func (p *Probe) Stop() {}

func (p *Probe) Close() error {
	p.server.GracefulStop()
	p.cancelFnc()

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
	traceEvent("Dispatching event %s", func() ([]byte, model.EventType, error) {
		eventJSON, err := serializers.MarshalEvent(event, p.resolvers)
		return eventJSON, event.GetEventType(), err
	})

	// send event to wildcard handlers, like the CWS rule engine, first
	p.sendEventToWildcardHandlers(event)

	// send event to specific event handlers, like the event monitor consumers, subsequently
	p.sendEventToSpecificEventTypeHandlers(event)
}

// Start the probe
func (p *Probe) Start() error {
	family, address := config.GetFamilyAddress(p.Config.RuntimeSecurity.EBPFLessSocket)

	conn, err := net.Listen(family, address)
	if err != nil {
		return err
	}

	go p.server.Serve(conn)

	seclog.Infof("starting listening for ebpf less events on : %s", p.Config.RuntimeSecurity.EBPFLessSocket)

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
	opts.normalize()

	ctx, cancel := context.WithCancel(context.Background())

	var grpcOpts []grpc.ServerOption
	p := &Probe{
		Opts:      opts,
		Config:    config,
		ctx:       ctx,
		cancelFnc: cancel,
		PlatformProbe: PlatformProbe{
			server: grpc.NewServer(grpcOpts...),
		},
	}

	ebpfless.RegisterSyscallMsgStreamServer(p.server, p)

	resolversOpts := resolvers.Opts{
		TagsResolver: opts.TagsResolver,
	}

	var err error
	p.resolvers, err = resolvers.NewResolvers(config, p.StatsdClient, p.scrubber, resolversOpts)
	if err != nil {
		return nil, err
	}

	p.fieldHandlers = &FieldHandlers{resolvers: p.resolvers}

	p.event = NewEvent(p.fieldHandlers)

	// be sure to zero the probe event before everything else
	p.zeroEvent()

	return p, nil
}

// HandleActions executes the actions of a triggered rule
func (p *Probe) HandleActions(_ *rules.Rule, _ eval.Event) {}

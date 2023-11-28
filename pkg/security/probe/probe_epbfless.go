// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"context"
	"errors"
	"net"
	"path/filepath"

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
	"github.com/DataDog/datadog-go/v5/statsd"
)

// EBPFLessProbe defines an eBPF less probe
type EBPFLessProbe struct {
	Resolvers *resolvers.EBPFLessResolvers

	// Constants and configuration
	opts         Opts
	config       *config.Config
	statsdClient statsd.ClientInterface

	// internals
	ebpfless.UnimplementedSyscallMsgStreamServer
	server        *grpc.Server
	seqNum        uint64
	probe         *Probe
	ctx           context.Context
	cancelFnc     context.CancelFunc
	fieldHandlers *EBPFLessFieldHandlers
}

// SendSyscallMsg handles gRPC messages
func (p *EBPFLessProbe) SendSyscallMsg(_ context.Context, syscallMsg *ebpfless.SyscallMsg) (*ebpfless.Response, error) {
	if p.seqNum != syscallMsg.SeqNum {
		seclog.Errorf("communication out of sync %d vs %d", p.seqNum, syscallMsg.SeqNum)
	}
	p.seqNum++

	event := p.probe.zeroEvent()

	switch syscallMsg.Type {
	case ebpfless.SyscallType_Exec:
		event.Type = uint32(model.ExecEventType)
		entry := p.Resolvers.ProcessResolver.AddExecEntry(syscallMsg.PID, syscallMsg.Exec.Filename, syscallMsg.Exec.Args, syscallMsg.Exec.Envs, syscallMsg.ContainerContext.ID)
		event.Exec.Process = &entry.Process
	case ebpfless.SyscallType_Fork:
		event.Type = uint32(model.ForkEventType)
		p.Resolvers.ProcessResolver.AddForkEntry(syscallMsg.PID, syscallMsg.Fork.PPID)
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
	event.ProcessCacheEntry = p.Resolvers.ProcessResolver.Resolve(syscallMsg.PID)
	if event.ProcessCacheEntry == nil {
		event.ProcessCacheEntry = model.NewPlaceholderProcessCacheEntry(syscallMsg.PID, syscallMsg.PID, false)
	}
	event.ProcessContext = &event.ProcessCacheEntry.ProcessContext

	p.DispatchEvent(event)

	return &ebpfless.Response{}, nil
}

// DispatchEvent sends an event to the probe event handler
func (p *EBPFLessProbe) DispatchEvent(event *model.Event) {
	traceEvent("Dispatching event %s", func() ([]byte, model.EventType, error) {
		eventJSON, err := serializers.MarshalEvent(event)
		return eventJSON, event.GetEventType(), err
	})

	// send event to wildcard handlers, like the CWS rule engine, first
	p.probe.sendEventToWildcardHandlers(event)

	// send event to specific event handlers, like the event monitor consumers, subsequently
	p.probe.sendEventToSpecificEventTypeHandlers(event)
}

// Init the probe
func (p *EBPFLessProbe) Init() error {
	if err := p.Resolvers.Start(p.ctx); err != nil {
		return err
	}

	return nil
}

// Stop the probe
func (p *EBPFLessProbe) Stop() {
	p.server.GracefulStop()
	p.cancelFnc()
}

// Close the probe
func (p *EBPFLessProbe) Close() error {
	return nil
}

// Start the probe
func (p *EBPFLessProbe) Start() error {
	family, address := config.GetFamilyAddress(p.config.RuntimeSecurity.EBPFLessSocket)

	conn, err := net.Listen(family, address)
	if err != nil {
		return err
	}

	go func() {
		_ = p.server.Serve(conn)
	}()

	seclog.Infof("starting listening for ebpf less events on : %s", p.config.RuntimeSecurity.EBPFLessSocket)

	return nil
}

// Snapshot the already exsisting entities
func (p *EBPFLessProbe) Snapshot() error {
	return nil
}

// Setup the probe
func (p *EBPFLessProbe) Setup() error {
	return nil
}

// OnNewDiscarder handles discarders
func (p *EBPFLessProbe) OnNewDiscarder(_ *rules.RuleSet, _ *model.Event, _ eval.Field, _ eval.EventType) {
}

// NewModel returns a new Model
func (p *EBPFLessProbe) NewModel() *model.Model {
	return NewEBPFLessModel()
}

// SendStats send the stats
func (p *EBPFLessProbe) SendStats() error {
	return nil
}

// DumpDiscarders dump the discarders
func (p *EBPFLessProbe) DumpDiscarders() (string, error) {
	return "", errors.New("not supported")
}

// FlushDiscarders flush the discarders
func (p *EBPFLessProbe) FlushDiscarders() error {
	return nil
}

// ApplyRuleSet applies the new ruleset
func (p *EBPFLessProbe) ApplyRuleSet(_ *rules.RuleSet) (*kfilters.ApplyRuleSetReport, error) {
	return &kfilters.ApplyRuleSetReport{}, nil
}

// HandleActions handles the rule actions
func (p *EBPFLessProbe) HandleActions(_ *rules.Rule, _ eval.Event) {}

// NewEvent returns a new event
func (p *EBPFLessProbe) NewEvent() *model.Event {
	return NewEBPFLessEvent(p.fieldHandlers)
}

// GetFieldHandlers returns the field handlers
func (p *EBPFLessProbe) GetFieldHandlers() model.FieldHandlers {
	return p.fieldHandlers
}

// DumpProcessCache dumps the process cache
func (p *EBPFLessProbe) DumpProcessCache(withArgs bool) (string, error) {
	return p.Resolvers.ProcessResolver.Dump(withArgs)
}

// AddDiscarderPushedCallback add a callback to the list of func that have to be called when a discarder is pushed to kernel
func (p *EBPFLessProbe) AddDiscarderPushedCallback(_ DiscarderPushedCallback) {}

// GetEventTags returns the event tags
func (p *EBPFLessProbe) GetEventTags(containerID string) []string {
	return p.Resolvers.TagsResolver.Resolve(containerID)
}

// NewEBPFLessProbe returns a new eBPF less probe
func NewEBPFLessProbe(probe *Probe, config *config.Config, opts Opts) (*EBPFLessProbe, error) {
	opts.normalize()

	ctx, cancelFnc := context.WithCancel(context.Background())

	var grpcOpts []grpc.ServerOption
	p := &EBPFLessProbe{
		probe:        probe,
		config:       config,
		opts:         opts,
		statsdClient: opts.StatsdClient,
		server:       grpc.NewServer(grpcOpts...),
		ctx:          ctx,
		cancelFnc:    cancelFnc,
	}

	ebpfless.RegisterSyscallMsgStreamServer(p.server, p)

	resolversOpts := resolvers.Opts{
		TagsResolver: opts.TagsResolver,
	}

	var err error
	p.Resolvers, err = resolvers.NewEBPFLessResolvers(config, p.statsdClient, probe.scrubber, resolversOpts)
	if err != nil {
		return nil, err
	}

	p.fieldHandlers = &EBPFLessFieldHandlers{resolvers: p.Resolvers}

	return p, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"context"
	"net"
	"path/filepath"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/security/config"
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
	probe *Probe

	// Constants and configuration
	opts         Opts
	config       *config.Config
	statsdClient statsd.ClientInterface

	// internals
	ebpfless.UnimplementedSyscallMsgStreamServer
	server    *grpc.Server
	seqNum    uint64
	resolvers *resolvers.Resolvers
	ctx       context.Context
	cancelFnc context.CancelFunc
}

func (p *EBPFLessProbe) SendSyscallMsg(ctx context.Context, syscallMsg *ebpfless.SyscallMsg) (*ebpfless.Response, error) {
	if p.seqNum != syscallMsg.SeqNum {
		seclog.Errorf("communication out of sync %d vs %d", p.seqNum, syscallMsg.SeqNum)
	}
	p.seqNum++

	event := p.probe.zeroEvent()

	switch syscallMsg.Type {
	case ebpfless.SyscallType_Exec:
		event.Type = uint32(model.ExecEventType)
		//entry := p.resolvers.ProcessResolver.AddExecEntry(syscallMsg.PID, syscallMsg.Exec.Filename, syscallMsg.Exec.Args, syscallMsg.Exec.Envs, syscallMsg.ContainerContext.ID)
		//event.Exec.Process = &entry.Process
	case ebpfless.SyscallType_Fork:
		event.Type = uint32(model.ForkEventType)
		//p.resolvers.ProcessResolver.AddForkEntry(syscallMsg.PID, syscallMsg.Fork.PPID)
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
	//	event.ProcessCacheEntry = p.resolvers.ProcessResolver.Resolve(syscallMsg.PID)
	if event.ProcessCacheEntry == nil {
		//		event.ProcessCacheEntry = model.NewPlaceholderProcessCacheEntry(syscallMsg.PID)
	}
	event.ProcessContext = &event.ProcessCacheEntry.ProcessContext

	p.DispatchEvent(event)

	return &ebpfless.Response{}, nil
}

// DispatchEvent sends an event to the probe event handler
func (p *EBPFLessProbe) DispatchEvent(event *model.Event) {
	traceEvent("Dispatching event %s", func() ([]byte, model.EventType, error) {
		eventJSON, err := serializers.MarshalEvent(event, p.resolvers)
		return eventJSON, event.GetEventType(), err
	})

	// send event to wildcard handlers, like the CWS rule engine, first
	p.probe.sendEventToWildcardHandlers(event)

	// send event to specific event handlers, like the event monitor consumers, subsequently
	p.probe.sendEventToSpecificEventTypeHandlers(event)
}

func (p *EBPFLessProbe) init() error {
	if err := p.resolvers.Start(p.ctx); err != nil {
		return err
	}

	return nil
}

func (p *EBPFLessProbe) close() error {
	p.server.GracefulStop()
	p.cancelFnc()

	return nil
}

func (p *EBPFLessProbe) start() error {
	family, address := config.GetFamilyAddress(p.config.RuntimeSecurity.EBPFLessSocket)

	conn, err := net.Listen(family, address)
	if err != nil {
		return err
	}

	go p.server.Serve(conn)

	seclog.Infof("starting listening for ebpf less events on : %s", p.config.RuntimeSecurity.EBPFLessSocket)

	return nil
}

func (p *Probe) sendStats() error {
	return nil
}

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

	/*resolversOpts := resolvers.Opts{
		TagsResolver: opts.TagsResolver,
	}*/

	//var err error
	// TODO safchain add platform specific resolvers
	/*p.resolvers, err = resolvers.NewResolvers(config, p.statsdClient, probe.scrubber, resolversOpts)
	if err != nil {
		return nil, err
	}*/

	probe.fieldHandlers = &FieldHandlers{resolvers: p.resolvers}

	probe.event = NewEvent(probe.fieldHandlers)

	// be sure to zero the probe event before everything else
	probe.zeroEvent()

	return p, nil
}

// HandleActions executes the actions of a triggered rule
func (p *Probe) HandleActions(_ *rules.Rule, _ eval.Event) {}

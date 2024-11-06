// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vmihailenco/msgpack/v5"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	maxMessageSize = 256 * 1024
)

type client struct {
	conn          net.Conn
	probe         *EBPFLessProbe
	nsID          uint64
	containerID   string
	containerName string
}

type clientMsg struct {
	ebpfless.Message
	*client
}

// EBPFLessProbe defines an eBPF less probe
type EBPFLessProbe struct {
	sync.Mutex

	Resolvers         *resolvers.EBPFLessResolvers
	containerContexts map[string]*ebpfless.ContainerContext

	// Constants and configuration
	opts         Opts
	config       *config.Config
	statsdClient statsd.ClientInterface

	// internals
	event         *model.Event
	server        *grpc.Server
	probe         *Probe
	ctx           context.Context
	cancelFnc     context.CancelFunc
	fieldHandlers *EBPFLessFieldHandlers
	clients       map[net.Conn]*client
	wg            sync.WaitGroup

	// kill action
	processKiller *ProcessKiller

	// hash action
	fileHasher *FileHasher
}

// GetProfileManager returns the Profile Managers
func (p *EBPFLessProbe) GetProfileManager() interface{} {
	return nil
}

func (p *EBPFLessProbe) handleClientMsg(cl *client, msg *ebpfless.Message) {
	switch msg.Type {
	case ebpfless.MessageTypeHello:
		if cl.nsID == 0 {
			p.probe.DispatchCustomEvent(
				NewEBPFLessHelloMsgEvent(msg.Hello, p.probe.scrubber),
			)

			cl.nsID = msg.Hello.NSID
			if msg.Hello.ContainerContext != nil {
				cl.containerID = msg.Hello.ContainerContext.ID
				cl.containerName = msg.Hello.ContainerContext.Name
				p.containerContexts[msg.Hello.ContainerContext.ID] = msg.Hello.ContainerContext
				seclog.Infof("tracing started for container ID [%s] (Name: [%s]) with entrypoint %q", msg.Hello.ContainerContext.ID, msg.Hello.ContainerContext.Name, msg.Hello.EntrypointArgs)
			}
		}
	case ebpfless.MessageTypeSyscall:
		p.handleSyscallMsg(cl, msg.Syscall)
	default:
		seclog.Errorf("unknown message type: %d", msg.Type)
	}
}

func copyFileAttributes(src *ebpfless.FileSyscallMsg, dst *model.FileEvent) {
	if strings.HasPrefix(src.Filename, "memfd:") {
		dst.SetPathnameStr("")
		dst.SetBasenameStr(src.Filename)
	} else {
		dst.SetPathnameStr(src.Filename)
		dst.SetBasenameStr(filepath.Base(src.Filename))
	}
	dst.CTime = src.CTime
	dst.MTime = src.MTime
	dst.Mode = uint16(src.Mode)
	dst.Inode = src.Inode
	if src.Credentials != nil {
		dst.UID = src.Credentials.UID
		dst.User = src.Credentials.User
		dst.GID = src.Credentials.GID
		dst.Group = src.Credentials.Group
	}
}

func (p *EBPFLessProbe) handleSyscallMsg(cl *client, syscallMsg *ebpfless.SyscallMsg) {
	event := p.zeroEvent()
	event.PIDContext.NSID = cl.nsID

	switch syscallMsg.Type {
	case ebpfless.SyscallTypeExec:
		event.Type = uint32(model.ExecEventType)

		var entry *model.ProcessCacheEntry
		if syscallMsg.Exec.FromProcFS {
			entry = p.Resolvers.ProcessResolver.AddProcFSEntry(
				process.CacheResolverKey{Pid: syscallMsg.PID, NSID: cl.nsID}, syscallMsg.Exec.PPID, syscallMsg.Exec.File.Filename,
				syscallMsg.Exec.Args, syscallMsg.Exec.ArgsTruncated, syscallMsg.Exec.Envs, syscallMsg.Exec.EnvsTruncated,
				syscallMsg.ContainerID, syscallMsg.Timestamp, syscallMsg.Exec.TTY)
		} else {
			entry = p.Resolvers.ProcessResolver.AddExecEntry(
				process.CacheResolverKey{Pid: syscallMsg.PID, NSID: cl.nsID}, syscallMsg.Exec.PPID, syscallMsg.Exec.File.Filename,
				syscallMsg.Exec.Args, syscallMsg.Exec.ArgsTruncated, syscallMsg.Exec.Envs, syscallMsg.Exec.EnvsTruncated,
				syscallMsg.ContainerID, syscallMsg.Timestamp, syscallMsg.Exec.TTY)
		}

		if syscallMsg.Exec.Credentials != nil {
			entry.Credentials.UID = syscallMsg.Exec.Credentials.UID
			entry.Credentials.EUID = syscallMsg.Exec.Credentials.EUID
			entry.Credentials.User = syscallMsg.Exec.Credentials.User
			entry.Credentials.EUser = syscallMsg.Exec.Credentials.EUser
			entry.Credentials.GID = syscallMsg.Exec.Credentials.GID
			entry.Credentials.EGID = syscallMsg.Exec.Credentials.EGID
			entry.Credentials.Group = syscallMsg.Exec.Credentials.Group
			entry.Credentials.EGroup = syscallMsg.Exec.Credentials.EGroup
		}
		event.Exec.Process = &entry.Process
		copyFileAttributes(&syscallMsg.Exec.File, &event.Exec.FileEvent)

	case ebpfless.SyscallTypeFork:
		event.Type = uint32(model.ForkEventType)
		p.Resolvers.ProcessResolver.AddForkEntry(process.CacheResolverKey{Pid: syscallMsg.PID, NSID: cl.nsID}, syscallMsg.Fork.PPID, syscallMsg.Timestamp)

	case ebpfless.SyscallTypeOpen:
		event.Type = uint32(model.FileOpenEventType)
		event.Open.Retval = syscallMsg.Retval
		copyFileAttributes(&syscallMsg.Open.FileSyscallMsg, &event.Open.File)
		event.Open.Mode = syscallMsg.Open.Mode
		event.Open.Flags = syscallMsg.Open.Flags

	case ebpfless.SyscallTypeSetUID:
		p.Resolvers.ProcessResolver.UpdateUID(process.CacheResolverKey{Pid: syscallMsg.PID, NSID: cl.nsID}, syscallMsg.SetUID.UID, syscallMsg.SetUID.EUID)
		event.Type = uint32(model.SetuidEventType)
		event.SetUID.UID = uint32(syscallMsg.SetUID.UID)
		event.SetUID.User = syscallMsg.SetUID.User
		event.SetUID.EUID = uint32(syscallMsg.SetUID.EUID)
		event.SetUID.EUser = syscallMsg.SetUID.EUser

	case ebpfless.SyscallTypeSetGID:
		p.Resolvers.ProcessResolver.UpdateGID(process.CacheResolverKey{Pid: syscallMsg.PID, NSID: cl.nsID}, syscallMsg.SetGID.GID, syscallMsg.SetGID.EGID)
		event.Type = uint32(model.SetgidEventType)
		event.SetGID.GID = uint32(syscallMsg.SetGID.GID)
		event.SetGID.Group = syscallMsg.SetGID.Group
		event.SetGID.EGID = uint32(syscallMsg.SetGID.EGID)
		event.SetGID.EGroup = syscallMsg.SetGID.EGroup

	case ebpfless.SyscallTypeSetFSUID:
		event.Type = uint32(model.SetuidEventType)
		event.SetUID.FSUID = uint32(syscallMsg.SetFSUID.FSUID)
		event.SetUID.FSUser = syscallMsg.SetFSUID.FSUser

	case ebpfless.SyscallTypeSetFSGID:
		event.Type = uint32(model.SetgidEventType)
		event.SetGID.FSGID = uint32(syscallMsg.SetFSGID.FSGID)
		event.SetGID.FSGroup = syscallMsg.SetFSGID.FSGroup

	case ebpfless.SyscallTypeCapset:
		event.Type = uint32(model.CapsetEventType)
		event.Capset.CapEffective = syscallMsg.Capset.Effective
		event.Capset.CapPermitted = syscallMsg.Capset.Permitted

	case ebpfless.SyscallTypeUnlink:
		event.Type = uint32(model.FileUnlinkEventType)
		event.Unlink.Retval = syscallMsg.Retval
		copyFileAttributes(&syscallMsg.Unlink.File, &event.Unlink.File)

	case ebpfless.SyscallTypeRmdir:
		event.Type = uint32(model.FileRmdirEventType)
		event.Rmdir.Retval = syscallMsg.Retval
		copyFileAttributes(&syscallMsg.Rmdir.File, &event.Rmdir.File)

	case ebpfless.SyscallTypeRename:
		event.Type = uint32(model.FileRenameEventType)
		event.Rename.Retval = syscallMsg.Retval
		copyFileAttributes(&syscallMsg.Rename.OldFile, &event.Rename.Old)
		copyFileAttributes(&syscallMsg.Rename.NewFile, &event.Rename.New)

	case ebpfless.SyscallTypeMkdir:
		event.Type = uint32(model.FileMkdirEventType)
		event.Mkdir.Retval = syscallMsg.Retval
		event.Mkdir.Mode = syscallMsg.Mkdir.Mode
		copyFileAttributes(&syscallMsg.Mkdir.Dir, &event.Mkdir.File)

	case ebpfless.SyscallTypeUtimes:
		event.Type = uint32(model.FileUtimesEventType)
		event.Utimes.Retval = syscallMsg.Retval
		event.Utimes.Atime = time.Unix(0, int64(syscallMsg.Utimes.ATime))
		event.Utimes.Mtime = time.Unix(0, int64(syscallMsg.Utimes.MTime))
		copyFileAttributes(&syscallMsg.Utimes.File, &event.Utimes.File)

	case ebpfless.SyscallTypeLink:
		event.Type = uint32(model.FileLinkEventType)
		event.Link.Retval = syscallMsg.Retval
		copyFileAttributes(&syscallMsg.Link.Target, &event.Link.Source)
		copyFileAttributes(&syscallMsg.Link.Link, &event.Link.Target)

	case ebpfless.SyscallTypeChmod:
		event.Type = uint32(model.FileChmodEventType)
		event.Chmod.Retval = syscallMsg.Retval
		event.Chmod.Mode = syscallMsg.Chmod.Mode
		copyFileAttributes(&syscallMsg.Chmod.File, &event.Chmod.File)

	case ebpfless.SyscallTypeChown:
		event.Type = uint32(model.FileChownEventType)
		event.Chown.Retval = syscallMsg.Retval
		event.Chown.UID = int64(syscallMsg.Chown.UID)
		event.Chown.User = syscallMsg.Chown.User
		event.Chown.GID = int64(syscallMsg.Chown.GID)
		event.Chown.Group = syscallMsg.Chown.Group
		copyFileAttributes(&syscallMsg.Chown.File, &event.Chown.File)

	case ebpfless.SyscallTypeUnloadModule:
		event.Type = uint32(model.UnloadModuleEventType)
		event.UnloadModule.Retval = syscallMsg.Retval
		event.UnloadModule.Name = syscallMsg.UnloadModule.Name

	case ebpfless.SyscallTypeLoadModule:
		event.Type = uint32(model.LoadModuleEventType)
		event.LoadModule.Retval = syscallMsg.Retval
		event.LoadModule.Name = syscallMsg.LoadModule.Name
		event.LoadModule.Args = syscallMsg.LoadModule.Args
		event.LoadModule.Argv = strings.Fields(syscallMsg.LoadModule.Args)
		event.LoadModule.LoadedFromMemory = syscallMsg.LoadModule.LoadedFromMemory
		if !syscallMsg.LoadModule.LoadedFromMemory {
			copyFileAttributes(&syscallMsg.LoadModule.File, &event.LoadModule.File)
		}

	case ebpfless.SyscallTypeChdir:
		event.Type = uint32(model.FileChdirEventType)
		event.Chdir.Retval = syscallMsg.Retval
		copyFileAttributes(&syscallMsg.Chdir.Dir, &event.Chdir.File)

	case ebpfless.SyscallTypeMount:
		event.Type = uint32(model.FileMountEventType)
		event.Mount.Retval = syscallMsg.Retval

		event.Mount.MountSourcePath = syscallMsg.Mount.Source
		event.Mount.MountPointPath = syscallMsg.Mount.Target
		event.Mount.MountPointStr = "/" + filepath.Base(syscallMsg.Mount.Target) // ??
		if syscallMsg.Mount.FSType == "bind" {
			event.Mount.FSType = utils.GetFSTypeFromFilePath(syscallMsg.Mount.Source)
		} else {
			event.Mount.FSType = syscallMsg.Mount.FSType
		}

	case ebpfless.SyscallTypeUmount:
		event.Type = uint32(model.FileUmountEventType)
		event.Umount.Retval = syscallMsg.Retval
	}

	// container context
	event.ContainerContext.ContainerID = containerutils.ContainerID(syscallMsg.ContainerID)
	if containerContext, exists := p.containerContexts[syscallMsg.ContainerID]; exists {
		event.ContainerContext.CreatedAt = containerContext.CreatedAt
		event.ContainerContext.Tags = []string{
			"image_name:" + containerContext.ImageShortName,
			"image_tag:" + containerContext.ImageTag,
		}
	}

	// copy span context if any
	if syscallMsg.SpanContext != nil {
		event.SpanContext.SpanID = syscallMsg.SpanContext.SpanID
		event.SpanContext.TraceID = syscallMsg.SpanContext.TraceID
	}

	// use ProcessCacheEntry process context as process context
	event.ProcessCacheEntry = p.Resolvers.ProcessResolver.Resolve(process.CacheResolverKey{Pid: syscallMsg.PID, NSID: cl.nsID})
	if event.ProcessCacheEntry == nil {
		event.ProcessCacheEntry = model.NewPlaceholderProcessCacheEntry(syscallMsg.PID, syscallMsg.PID, false)
	}
	event.ProcessContext = &event.ProcessCacheEntry.ProcessContext

	if syscallMsg.Type == ebpfless.SyscallTypeExit {
		event.Type = uint32(model.ExitEventType)
		event.ProcessContext.ExitTime = time.Unix(0, int64(syscallMsg.Timestamp))
		event.Exit.Process = &event.ProcessCacheEntry.Process
		event.Exit.Cause = uint32(syscallMsg.Exit.Cause)
		event.Exit.Code = syscallMsg.Exit.Code
		defer p.Resolvers.ProcessResolver.DeleteEntry(process.CacheResolverKey{Pid: syscallMsg.PID, NSID: cl.nsID}, event.ProcessContext.ExitTime)

		// update action reports
		p.processKiller.HandleProcessExited(event)
		p.fileHasher.HandleProcessExited(event)
	}

	p.DispatchEvent(event)

	// flush pending actions
	p.processKiller.FlushPendingReports()
	p.fileHasher.FlushPendingReports()
}

// DispatchEvent sends an event to the probe event handler
func (p *EBPFLessProbe) DispatchEvent(event *model.Event) {
	traceEvent("Dispatching event %s", func() ([]byte, model.EventType, error) {
		eventJSON, err := serializers.MarshalEvent(event, nil)
		return eventJSON, event.GetEventType(), err
	})

	// send event to wildcard handlers, like the CWS rule engine, first
	p.probe.sendEventToHandlers(event)

	// send event to specific event handlers, like the event monitor consumers, subsequently
	p.probe.sendEventToConsumers(event)
}

// Init the probe
func (p *EBPFLessProbe) Init() error {
	p.processKiller.Start(p.ctx, &p.wg)

	if err := p.Resolvers.Start(p.ctx); err != nil {
		return err
	}

	return nil
}

// Stop the probe
func (p *EBPFLessProbe) Stop() {
	p.server.GracefulStop()

	p.Lock()
	for conn := range p.clients {
		conn.Close()
	}
	p.Unlock()

	p.cancelFnc()

	p.wg.Wait()
}

// Close the probe
func (p *EBPFLessProbe) Close() error {
	p.Lock()
	defer p.Unlock()

	for conn := range p.clients {
		conn.Close()
		delete(p.clients, conn)
	}

	return nil
}

func (p *EBPFLessProbe) readMsg(conn net.Conn, msg *ebpfless.Message) error {
	sizeBuf := make([]byte, 4)

	n, err := conn.Read(sizeBuf)
	if err != nil {
		return err
	}

	if n < 4 {
		// TODO return EOF
		return errors.New("not enough data")
	}

	size := binary.NativeEndian.Uint32(sizeBuf)
	if size > maxMessageSize {
		return fmt.Errorf("data overflow the max size: %d", size)
	}

	buf := make([]byte, size)

	var read uint32
	for read < size {
		n, err = conn.Read(buf[read:size])
		if err != nil {
			return err
		}
		read += uint32(n)
	}

	return msgpack.Unmarshal(buf[0:size], msg)
}

// GetClientsCount returns the number of connected clients
func (p *EBPFLessProbe) GetClientsCount() int {
	p.Lock()
	defer p.Unlock()
	return len(p.clients)
}

func (p *EBPFLessProbe) handleNewClient(conn net.Conn, ch chan clientMsg) {
	client := &client{
		conn:  conn,
		probe: p,
	}

	p.Lock()
	p.clients[conn] = client
	p.Unlock()

	seclog.Debugf("new connection from: %v", conn.RemoteAddr())

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		msg := clientMsg{
			client: client,
		}
		for {

			msg.Reset()
			if err := p.readMsg(conn, &msg.Message); err != nil {
				if errors.Is(err, io.EOF) {
					seclog.Warnf("connection closed by client: %v", conn.RemoteAddr())
				} else {
					seclog.Warnf("error while reading message: %v", err)
				}

				p.Lock()
				delete(p.clients, conn)
				p.Unlock()
				conn.Close()

				msg.Type = ebpfless.MessageTypeGoodbye
				ch <- msg

				return
			}

			ch <- msg
		}
	}()
}

// Start the probe
func (p *EBPFLessProbe) Start() error {
	family, address := config.GetFamilyAddress(p.config.RuntimeSecurity.EBPFLessSocket)
	_ = family

	tcpAddr, err := net.ResolveTCPAddr("tcp4", address)
	if err != nil {
		return err
	}

	// Start listening for TCP connections on the given address
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return err
	}

	ch := make(chan clientMsg, 100)

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-p.ctx.Done():
					return
				default:
					seclog.Errorf("unable to accept new connection: %s", err)
					continue
				}
			}
			p.handleNewClient(conn, ch)
		}
	}()

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		for {
			select {
			case <-p.ctx.Done():
				listener.Close()

				return
			case msg := <-ch:
				if msg.Type == ebpfless.MessageTypeGoodbye {
					if msg.client.containerID != "" {
						delete(p.containerContexts, msg.client.containerID)
						seclog.Infof("tracing stopped for container ID [%s] (Name: [%s])", msg.client.containerID, msg.client.containerName)
					}
					continue
				}
				p.handleClientMsg(msg.client, &msg.Message)
			}
		}
	}()

	seclog.Infof("starting listening for ebpf less events on : %s", p.config.RuntimeSecurity.EBPFLessSocket)

	return nil
}

// Snapshot the already existing entities
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
	p.processKiller.SendStats(p.statsdClient)
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

// OnNewRuleSetLoaded resets statistics and states once a new rule set is loaded
func (p *EBPFLessProbe) OnNewRuleSetLoaded(rs *rules.RuleSet) {
	p.processKiller.Reset(rs)
}

// HandleActions handles the rule actions
func (p *EBPFLessProbe) HandleActions(ctx *eval.Context, rule *rules.Rule) {
	ev := ctx.Event.(*model.Event)

	for _, action := range rule.Actions {
		if !action.IsAccepted(ctx) {
			continue
		}

		switch {
		case action.Def.Kill != nil:
			// do not handle kill action on event with error
			if ev.Error != nil {
				return
			}

			if p.processKiller.KillAndReport(action.Def.Kill, rule, ev, func(pid uint32, sig uint32) error {
				return p.processKiller.KillFromUserspace(pid, sig, ev)
			}) {
				p.probe.onRuleActionPerformed(rule, action.Def)
			}
		case action.Def.Hash != nil:
			if p.fileHasher.HashAndReport(rule, ev) {
				p.probe.onRuleActionPerformed(rule, action.Def)
			}
		}
	}
}

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

func (p *EBPFLessProbe) zeroEvent() *model.Event {
	p.event.Zero()
	p.event.FieldHandlers = p.fieldHandlers
	p.event.Origin = EBPFLessOrigin
	return p.event
}

// EnableEnforcement sets the enforcement mode
func (p *EBPFLessProbe) EnableEnforcement(state bool) {
	p.processKiller.SetState(state)
}

// NewEBPFLessProbe returns a new eBPF less probe
func NewEBPFLessProbe(probe *Probe, config *config.Config, opts Opts, telemetry telemetry.Component) (*EBPFLessProbe, error) {
	opts.normalize()

	processKiller, err := NewProcessKiller(config)
	if err != nil {
		return nil, err
	}

	ctx, cancelFnc := context.WithCancel(context.Background())

	var grpcOpts []grpc.ServerOption
	p := &EBPFLessProbe{
		probe:             probe,
		config:            config,
		opts:              opts,
		statsdClient:      opts.StatsdClient,
		server:            grpc.NewServer(grpcOpts...),
		ctx:               ctx,
		cancelFnc:         cancelFnc,
		clients:           make(map[net.Conn]*client),
		processKiller:     processKiller,
		containerContexts: make(map[string]*ebpfless.ContainerContext),
	}

	resolversOpts := resolvers.Opts{
		TagsResolver: opts.TagsResolver,
	}

	p.Resolvers, err = resolvers.NewEBPFLessResolvers(config, p.statsdClient, probe.scrubber, resolversOpts, telemetry)
	if err != nil {
		return nil, err
	}

	p.fileHasher = NewFileHasher(config, p.Resolvers.HashResolver)

	hostname, err := utils.GetHostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}

	p.fieldHandlers = &EBPFLessFieldHandlers{config: config, resolvers: p.Resolvers, hostname: hostname}

	p.event = p.NewEvent()

	// be sure to zero the probe event before everything else
	p.zeroEvent()

	return p, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ConsumerID is the ID of the YARA exec consumer
const ConsumerID = "YARA_EXEC_SCANNER"

// defaultChanSize is used when the configured channel size isn't positive
const defaultChanSize = 500

// ExecHandler receives every ExecFile extracted from an exec event. It runs on the consumer's
// goroutine, off the event hot path, and is called sequentially: it doesn't need to be safe for
// concurrent use, but it must not block for long or the consumer channel fills up and drops events.
type ExecHandler func(ExecFile)

// ExecConsumer is an event monitor consumer of exec events. Copy() builds the ExecFile entries
// of the executed files on the event hot path, and HandleEvent() passes them to an ExecHandler
// on the consumer's own goroutine.
type ExecConsumer struct {
	chanSize int
	stats    *Stats
	handler  ExecHandler
	now      func() time.Time
	// lifecycle, when set, is started and stopped with the consumer: it runs the stages behind
	// the handler (see Pipeline)
	lifecycle lifecycle
}

// lifecycle is started by the consumer's Start, and stopped by its Stop
type lifecycle interface {
	Start() error
	Stop()
}

// execFiles is what Copy() hands to HandleEvent(). It holds the executed file, and the
// interpreter for interpreter execs, in a single allocation.
type execFiles struct {
	// main is the executed file: the binary, or the script of an interpreter exec
	main ExecFile
	// interpreter is the script interpreter of an interpreter exec, when hasInterpreter is set
	interpreter    ExecFile
	hasInterpreter bool
}

var _ eventmonitor.EventConsumerHandler = (*ExecConsumer)(nil)
var _ eventmonitor.EventConsumer = (*ExecConsumer)(nil)

// NewExecConsumer creates an ExecConsumer and registers it with the event monitor. stats and
// handler must not be nil. This function should be called with the EventMonitor instance created
// in cmd/system-probe/modules/eventmonitor.go:createEventMonitorModule.
func NewExecConsumer(evm *eventmonitor.EventMonitor, cfg *Config, stats *Stats, handler ExecHandler) (*ExecConsumer, error) {
	c, err := newExecConsumer(cfg, stats, handler)
	if err != nil {
		return nil, err
	}
	if err := c.register(evm); err != nil {
		return nil, err
	}
	return c, nil
}

// register adds the consumer to the event monitor, as an event handler and as a consumer (for
// Start and Stop)
func (c *ExecConsumer) register(evm *eventmonitor.EventMonitor) error {
	if err := evm.AddEventConsumerHandler(c); err != nil {
		return fmt.Errorf("cannot add yara event consumer handler: %w", err)
	}
	evm.RegisterEventConsumer(c)
	return nil
}

func newExecConsumer(cfg *Config, stats *Stats, handler ExecHandler) (*ExecConsumer, error) {
	if stats == nil {
		return nil, errors.New("yara consumer: nil stats")
	}
	if handler == nil {
		return nil, errors.New("yara consumer: nil handler")
	}

	chanSize := defaultChanSize
	if cfg != nil && cfg.ChanSize > 0 {
		chanSize = cfg.ChanSize
	}

	return &ExecConsumer{
		chanSize: chanSize,
		stats:    stats,
		handler:  handler,
		now:      time.Now,
	}, nil
}

// --- eventmonitor.EventConsumer interface methods

// ID returns the ID of the consumer
func (c *ExecConsumer) ID() string {
	return ConsumerID
}

// Start starts the consumer. The event monitor runs HandleEvent on its own goroutine; Start only
// starts the pipeline stages behind the handler, if any.
func (c *ExecConsumer) Start() error {
	if c.lifecycle != nil {
		return c.lifecycle.Start()
	}
	return nil
}

// Stop stops the consumer, and the pipeline stages behind the handler, if any. The event monitor
// calls it after the probe has stopped, and waited for, the goroutine running HandleEvent: no
// handler call runs concurrently with it.
func (c *ExecConsumer) Stop() {
	if c.lifecycle != nil {
		c.lifecycle.Stop()
	}
}

// --- eventmonitor.EventConsumerHandler interface methods

// ChanSize returns the size of the channel the event monitor uses to send events to this consumer
func (c *ExecConsumer) ChanSize() int {
	return c.chanSize
}

// EventTypes returns the event types this consumer handles
func (c *ExecConsumer) EventTypes() []model.EventType {
	return []model.EventType{model.ExecEventType}
}

// Copy runs on the event hot path and does no file I/O. It returns nil, so the event is not sent
// to the consumer, for kworkers and events without a process.
//
// Paths are resolved lazily on CWS events, so Copy resolves them through the field handlers (the
// dentry resolver, as rule evaluation does). Without a path, the reader can only use
// /proc/<pid>/exe, which is gone for short-lived processes and wrong for scripts.
//
// For an interpreter exec (#! script), Process.FileEvent is the script, i.e. the executed file,
// and LinuxBinprm.FileEvent is the interpreter; /proc/<pid>/exe points at the interpreter. The
// script entry must therefore be flagged IsScript, so that the reader doesn't read the
// interpreter through /proc/<pid>/exe and report it under the script's identity.
func (c *ExecConsumer) Copy(ev *model.Event) any {
	if ev.GetEventType() != model.ExecEventType {
		return nil
	}
	p := ev.Exec.Process
	if p == nil || p.IsKworker {
		return nil
	}

	now := c.now()
	files := &execFiles{
		main: newExecFile(ev, p, &p.FileEvent, now),
	}
	// the executed file's Filesystem is resolved by the process resolver before dispatch
	files.main.Filesystem = p.FileEvent.Filesystem

	if p.HasInterpreter() {
		files.main.IsScript = true

		// the interpreter is a regular binary entry: /proc/<pid>/exe is the interpreter itself.
		// The process resolver doesn't resolve its Filesystem, and doing it here would require a
		// mount resolver lookup, so it is left empty: the FUSE/NFS identity cache bypass doesn't
		// apply to interpreters; accepted for the PoC (see FileReader.Process).
		files.interpreter = newExecFile(ev, p, &p.LinuxBinprm.FileEvent, now)
		files.hasInterpreter = true
	}

	return files
}

// newExecFile copies the fields of fe, a file of process p, into an ExecFile
func newExecFile(ev *model.Event, p *model.Process, fe *model.FileEvent, now time.Time) ExecFile {
	path := fe.PathnameStr
	if path == "" && ev.FieldHandlers != nil {
		path = ev.FieldHandlers.ResolveFilePath(ev, fe)
	}
	return ExecFile{
		PID:         p.Pid,
		CGroupID:    p.CGroup.CGroupID,
		ContainerID: p.ContainerContext.ContainerID,
		Path:        path,
		MountID:     fe.MountID,
		Inode:       fe.Inode,
		CTime:       fe.CTime,
		SeenAt:      now,
	}
}

// HandleEvent runs on the consumer's goroutine and passes the ExecFile entries to the handler
func (c *ExecConsumer) HandleEvent(ev any) {
	files, ok := ev.(*execFiles)
	if !ok {
		return
	}

	c.stats.ExecsReceived.Add(1)

	c.handler(files.main)
	if files.hasInterpreter {
		c.handler(files.interpreter)
	}
}

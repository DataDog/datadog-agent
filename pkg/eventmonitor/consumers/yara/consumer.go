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
}

// execFiles is what Copy() hands to HandleEvent(). It holds the main binary, and the script file
// for interpreter execs, in a single allocation.
type execFiles struct {
	main      ExecFile
	script    ExecFile
	hasScript bool
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

	if err := evm.AddEventConsumerHandler(c); err != nil {
		return nil, fmt.Errorf("cannot add yara event consumer handler: %w", err)
	}
	evm.RegisterEventConsumer(c)

	return c, nil
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

// Start starts the consumer. The event monitor runs HandleEvent on its own goroutine, so there is
// nothing to start here.
func (c *ExecConsumer) Start() error {
	return nil
}

// Stop stops the consumer
func (c *ExecConsumer) Stop() {
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

// Copy runs on the event hot path: it only copies fields that are already resolved on the event,
// and does no I/O. It returns nil, so the event is not sent to the consumer, for kworkers and
// events without a process.
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
		main: newExecFile(p, &p.FileEvent, now),
	}
	// the main binary's Filesystem is resolved by the process resolver before dispatch
	files.main.Filesystem = p.FileEvent.Filesystem

	if p.HasInterpreter() {
		files.script = newExecFile(p, &p.LinuxBinprm.FileEvent, now)
		// the process resolver doesn't resolve the script's Filesystem, and doing it here would
		// require a mount resolver lookup, so it is left empty
		files.script.IsScript = true
		files.hasScript = true
	}

	return files
}

// newExecFile copies the fields of fe, a file of process p, into an ExecFile
func newExecFile(p *model.Process, fe *model.FileEvent, now time.Time) ExecFile {
	return ExecFile{
		PID:         p.Pid,
		CGroupID:    p.CGroup.CGroupID,
		ContainerID: p.ContainerContext.ContainerID,
		Path:        fe.PathnameStr,
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
	if files.hasScript {
		c.handler(files.script)
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	logutil "github.com/DataDog/datadog-agent/pkg/util/log"
	ddos "github.com/DataDog/datadog-agent/pkg/util/os"
)

const eventConsumerSubsystem = "sender__event_consumer"

var eventConsumerTelemetry = struct {
	eventsReceived telemetry.Counter
	processCount   telemetry.Gauge
}{
	telemetry.NewCounter(eventConsumerSubsystem, "events_received", []string{"event_type"}, ""),
	telemetry.NewGauge(eventConsumerSubsystem, "process_count", nil, ""),
}

var _ eventmonitor.EventConsumerHandler = &directSenderConsumer{}
var _ eventmonitor.EventConsumer = &directSenderConsumer{}

// directSenderConsumerInstance contains the instance of the direct sender consumer (if there is one).
// this is necessary due to the out-of-order initialization between CNM and Event Monitor.
var directSenderConsumerInstance atomic.Pointer[directSenderConsumer]

type directSenderConsumer struct {
	log       log.Component
	processes map[uint32]*process
	mtx       sync.Mutex

	proxyFilter  *dockerProxyFilter
	extractor    *serviceExtractor
	pidAliveFunc func(pid int) bool
}

// NewDirectSenderConsumer creates the direct sender consumer and returns it for event monitor registration
func NewDirectSenderConsumer(em EventConsumerRegistry, log log.Component, sysprobeconfig sysprobeconfig.Component) (eventmonitor.EventConsumer, error) {
	dsc := &directSenderConsumer{
		log:          log,
		processes:    make(map[uint32]*process),
		proxyFilter:  newDockerProxyFilter(log),
		extractor:    newServiceExtractor(sysprobeconfig),
		pidAliveFunc: ddos.PidExists,
	}
	err := em.AddEventConsumerHandler(dsc)
	if err != nil {
		return nil, err
	}
	directSenderConsumerInstance.Store(dsc)
	return dsc, nil
}

// ID implements eventmonitor.EventConsumer and eventmonitor.EventConsumerHandler
func (d *directSenderConsumer) ID() string {
	return "networkdirectsender"
}

// ChanSize implements eventmonitor.EventConsumerHandler
func (d *directSenderConsumer) ChanSize() int {
	return 100
}

type process struct {
	Pid       uint32
	PPid      uint32
	Cmdline   []string
	Cwd       string
	EventType model.EventType
}

// EventTypes implements eventmonitor.EventConsumerHandler
func (d *directSenderConsumer) EventTypes() []model.EventType {
	return []model.EventType{
		model.ExecEventType,
		model.ExitEventType,
		model.ForkEventType,
	}
}

// Start implements eventmonitor.EventConsumer
func (d *directSenderConsumer) Start() error {
	return nil
}

// Stop implements eventmonitor.EventConsumer
func (d *directSenderConsumer) Stop() {}

// Copy implements eventmonitor.EventConsumerHandler
func (d *directSenderConsumer) Copy(ev *model.Event) any {
	p := &process{
		Pid:       ev.GetProcessPid(),
		PPid:      ev.GetProcessPpid(),
		EventType: ev.GetEventType(),
		Cmdline:   ev.GetExecCmdargv(),
	}
	return p
}

var cwdLogLimiter = logutil.NewLogLimit(20, 10*time.Minute)

// HandleEvent implements eventmonitor.EventConsumerHandler
func (d *directSenderConsumer) HandleEvent(ev any) {
	p, ok := ev.(*process)
	if !ok {
		return
	}
	eventConsumerTelemetry.eventsReceived.Inc(p.EventType.String())
	if p.EventType == model.ExecEventType || p.EventType == model.ForkEventType {
		cwd, err := os.Readlink(kernel.HostProc(strconv.Itoa(int(p.Pid)), "cwd"))
		if err != nil && !os.IsNotExist(err) {
			if cwdLogLimiter.ShouldLog() {
				d.log.Warnf("error reading working directory for pid %d: %s", p.Pid, err)
			}
		}
		p.Cwd = cwd
	}
	d.process(p)
	d.proxyFilter.process(p)
	d.extractor.process(p)
}

func (d *directSenderConsumer) process(p *process) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	if p.EventType == model.ForkEventType && p.PPid > 0 {
		if parent, ok := d.processes[p.PPid]; ok && parent != nil {
			p.Cmdline = parent.Cmdline
		}
	}

	if _, seen := d.processes[p.Pid]; seen {
		if p.EventType == model.ExitEventType {
			// mark process as dead so it will be removed after next set of connections are collected
			d.processes[p.Pid] = nil
		}
	}

	if p.EventType == model.ExecEventType || p.EventType == model.ForkEventType {
		d.processes[p.Pid] = p
	}
	eventConsumerTelemetry.processCount.Set(float64(len(d.processes)))
}

// cleanupProcesses is called after connections have been collected, so stale process entries can be cleaned up.
func (d *directSenderConsumer) cleanupProcesses() {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	for pid, p := range d.processes {
		alive := p != nil
		if alive {
			alive = d.pidAliveFunc(int(pid))
		}

		if !alive {
			d.extractor.handleDeadProcess(pid)
			delete(d.processes, pid)
		}
	}
	eventConsumerTelemetry.processCount.Set(float64(len(d.processes)))
}

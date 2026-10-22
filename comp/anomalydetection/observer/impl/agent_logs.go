// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logging"
	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logsfilter"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/log/observer"
)

const (
	agentLogSource = "datadog-agent"
	// This queue only absorbs short logger bursts; Observer's own ingress queue
	// remains the backlog boundary. Keeping this small bounds retained log data.
	agentLogTapQueueSize = 100
)

type queuedAgentLog struct {
	level       pkglog.LogLevel
	message     string
	component   string
	timestampMs int64
}

// agentLogTap keeps all non-trivial observer work out of pkg/util/log's global
// logger lock. The logger callback only applies cheap safety gates and attempts
// a non-blocking enqueue; this worker constructs tags and JSON and forwards the
// log into the observer, which owns processing rules and rate limiting.
type agentLogTap struct {
	handle         observerdef.Handle
	minBucket      logsfilter.PriorityBucket
	onQueueDropped func(count uint64)

	queue chan queuedAgentLog

	queueMu  sync.RWMutex
	stopped  bool
	stopOnce sync.Once
	worker   sync.WaitGroup

	queuedDrops atomic.Uint64
}

// installAgentLogTap registers a pkg/util/log observer that forwards
// agent-internal log lines into the observer pipeline. The returned tap must be
// stopped during component shutdown.
//
// Logs below minSeverity are dropped in the logger callback before enqueueing.
// onQueueDropped is called from the worker with the number of logs dropped when
// the tap queue was full and may be nil.
func installAgentLogTap(
	handle observerdef.Handle,
	minSeverity string,
	onQueueDropped func(count uint64),
) *agentLogTap {
	return installAgentLogTapWithQueueSize(
		handle,
		minSeverity,
		onQueueDropped,
		agentLogTapQueueSize,
	)
}

func installAgentLogTapWithQueueSize(
	handle observerdef.Handle,
	minSeverity string,
	onQueueDropped func(count uint64),
	queueSize int,
) *agentLogTap {
	tap := &agentLogTap{
		handle:         handle,
		minBucket:      logsfilter.MinBucketForSeverity(minSeverity),
		onQueueDropped: onQueueDropped,
		queue:          make(chan queuedAgentLog, queueSize),
	}

	tap.worker.Add(1)
	go tap.run()
	observer.SetLogObserver(tap.enqueue)
	return tap
}

// enqueue blocks handling of log messages, so keep this path bounded: no tag
// construction, rule matching, rate limiter, JSON encoding, or observer call
// is allowed here.
func (t *agentLogTap) enqueue(level pkglog.LogLevel, message string) {
	// Prevent anomaly-detection's own logs from feeding back into the tap. This
	// must happen before the asynchronous handoff because worker-side logging is
	// no longer covered by pkg/util/log's synchronous recursion guard.
	if strings.HasPrefix(message, logging.Prefix) {
		return
	}

	// LogLevel numeric values deliberately match PriorityBucket values.
	if logsfilter.PriorityBucket(level) < t.minBucket {
		return
	}

	entry := queuedAgentLog{
		level:       level,
		message:     message,
		component:   observer.GetLoggerName(),
		timestampMs: time.Now().UnixMilli(),
	}

	// The read lock only contends during shutdown and prevents close(queue) from
	// racing a producer that has already loaded the observer callback.
	t.queueMu.RLock()
	defer t.queueMu.RUnlock()
	if t.stopped {
		return
	}
	select {
	case t.queue <- entry:
	default:
		t.queuedDrops.Add(1)
	}
}

func (t *agentLogTap) run() {
	defer t.worker.Done()
	for entry := range t.queue {
		t.process(entry)
		t.reportQueueDrops()
	}
	t.reportQueueDrops()
}

func (t *agentLogTap) process(entry queuedAgentLog) {
	status := entry.level.String()
	tags := make([]string, 0, 3)
	tags = append(tags, "source:"+agentLogSource)
	if entry.component != "" {
		tags = append(tags, "component:"+entry.component)
	}
	tags = append(tags, "level:"+status)

	payload, _ := json.Marshal(map[string]any{
		"msg": entry.message,
	})
	t.handle.ObserveLog(&agentLogView{
		content:     string(payload),
		status:      status,
		tags:        tags,
		hostname:    "",
		timestampMs: entry.timestampMs,
	})
}

func (t *agentLogTap) reportQueueDrops() {
	count := t.queuedDrops.Swap(0)
	if count > 0 && t.onQueueDropped != nil {
		t.onQueueDropped(count)
	}
}

// stop disables the process-wide hook, safely closes the queue after any
// in-flight callback has left its enqueue critical section, and drains all logs
// that were accepted before shutdown.
func (t *agentLogTap) stop() {
	t.stopOnce.Do(func() {
		observer.SetLogObserver(nil)
		t.queueMu.Lock()
		t.stopped = true
		close(t.queue)
		t.queueMu.Unlock()
		t.worker.Wait()
	})
}

// agentLogView is a minimal observerdef.LogView implementation for agent-internal logs.
type agentLogView struct {
	content     string
	status      string
	tags        []string
	hostname    string
	timestampMs int64
}

func (v *agentLogView) GetSource() string            { return agentLogSource }
func (v *agentLogView) GetContent() string           { return v.content }
func (v *agentLogView) GetStatus() string            { return v.status }
func (v *agentLogView) Tags() []string               { return v.tags }
func (v *agentLogView) GetHostname() string          { return v.hostname }
func (v *agentLogView) GetTimestampUnixMilli() int64 { return v.timestampMs }

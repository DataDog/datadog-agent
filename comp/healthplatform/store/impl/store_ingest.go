// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package storeimpl

import (
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
)

const (
	ingestChanBuf = 1024
	flushInterval = time.Second
)

type cmdKind uint8

const (
	cmdReport cmdKind = iota
	cmdResolve
	cmdResolveAll
	cmdSync
)

type ingestCmd struct {
	kind  cmdKind
	issue *healthplatform.Issue
	id    string
	done  chan struct{}
}

func (cmd ingestCmd) opLabel() string {
	switch cmd.kind {
	case cmdReport:
		return "report"
	case cmdResolve:
		return "resolve"
	case cmdResolveAll:
		return "resolve_all"
	default:
		return "sync"
	}
}

func (h *healthPlatformImpl) enqueue(cmd ingestCmd) {
	h.ingestMu.Lock()
	h.ingestQueue = append(h.ingestQueue, cmd)
	droppedLabel := ""
	if len(h.ingestQueue) > ingestChanBuf {
		for i, queued := range h.ingestQueue {
			if queued.kind == cmdReport {
				copy(h.ingestQueue[i:], h.ingestQueue[i+1:])
				h.ingestQueue[len(h.ingestQueue)-1] = ingestCmd{}
				h.ingestQueue = h.ingestQueue[:len(h.ingestQueue)-1]
				droppedLabel = queued.opLabel()
				h.metrics.issuesDropped.Inc(droppedLabel)
				break
			}
		}
	}
	h.metrics.ingestQueueLen.Set(float64(len(h.ingestQueue)))
	h.ingestMu.Unlock()
	select {
	case h.ingestWake <- struct{}{}:
	default:
	}
	if droppedLabel != "" && h.dropLogLimit.ShouldLog() {
		h.log.Warnf("health platform: ingest queue full, dropping %s operation", droppedLabel)
	}
}

func (h *healthPlatformImpl) run() {
	defer close(h.doneCh)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-h.ingestWake:
			h.drain(true, ticker.C)
		case <-ticker.C:
			h.flushIfDirty()
		case <-h.stopCh:
			h.drain(false, nil)
			h.flushIfDirty()
			return
		}
	}
}

func (h *healthPlatformImpl) drain(enrich bool, ticks <-chan time.Time) {
	for {
		if enrich {
			select {
			case <-h.stopCh:
				return
			case <-ticks:
				h.flushIfDirty()
			default:
			}
		}
		h.ingestMu.Lock()
		if len(h.ingestQueue) == 0 {
			h.ingestMu.Unlock()
			return
		}
		cmd := h.ingestQueue[0]
		h.ingestQueue[0] = ingestCmd{}
		h.ingestQueue = h.ingestQueue[1:]
		if len(h.ingestQueue) == 0 {
			h.ingestQueue = nil
		}
		h.metrics.ingestQueueLen.Set(float64(len(h.ingestQueue)))
		h.ingestMu.Unlock()
		h.apply(cmd, enrich)
	}
}

func (h *healthPlatformImpl) apply(cmd ingestCmd, enrich bool) {
	switch cmd.kind {
	case cmdReport:
		h.applyReport(cmd.issue, enrich)
	case cmdResolve:
		h.applyResolve(cmd.id)
	case cmdResolveAll:
		h.applyResolveAll()
	case cmdSync:
	}
	if cmd.done != nil {
		close(cmd.done)
	}
}

func (h *healthPlatformImpl) flushIfDirty() {
	if !h.dirty {
		return
	}
	if err := h.saveToDisk(); err != nil {
		h.log.Warn("Failed to persist issues to disk: " + err.Error())
		return
	}
	h.dirty = false
}

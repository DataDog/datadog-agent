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
	select {
	case h.ingestCh <- cmd:
	default:
		label := cmd.opLabel()
		h.metrics.issuesDropped.Inc(label)
		if h.dropLogLimit.ShouldLog() {
			h.log.Warnf("health platform: ingest queue full, dropping %s operation", label)
		}
	}
}

func (h *healthPlatformImpl) run() {
	defer close(h.doneCh)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		h.metrics.ingestQueueLen.Set(float64(len(h.ingestCh)))
		select {
		case cmd := <-h.ingestCh:
			h.apply(cmd, true)
		case <-ticker.C:
			h.flushIfDirty()
		case <-h.stopCh:
			for {
				h.metrics.ingestQueueLen.Set(float64(len(h.ingestCh)))
				select {
				case cmd := <-h.ingestCh:
					h.apply(cmd, false)
				default:
					h.flushIfDirty()
					return
				}
			}
		}
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
	}
	h.dirty = false
}

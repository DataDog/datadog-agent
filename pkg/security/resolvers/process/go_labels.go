// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package process holds process related files
package process

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/security/golabels"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func (p *EBPFResolver) queueGoLabels(pid uint32) {
	if !p.config.SpanTrackingEnabled {
		return
	}
	if p.goLabelsMap == nil {
		p.reportSpanCtx(spanCtxStepGoLabels, pid, fmt.Errorf("%w: go_labels_procs map not available", errSpanCtxMapError))
		return
	}

	p.goLabelsLock.Lock()
	defer p.goLabelsLock.Unlock()

	if _, pending := p.goLabelsPending[pid]; pending {
		return
	}

	select {
	case p.goLabelsQueue <- pid:
		p.goLabelsPending[pid] = struct{}{}
	default:
		p.countSpanCtx(spanCtxStepGoLabels, spanCtxQueueFull)
		seclog.Warnf("Go labels queue full, dropping pid %d", pid)
	}
}

func (p *EBPFResolver) resolveGoLabelsLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case pid := <-p.goLabelsQueue:
			p.goLabelsLock.Lock()
			delete(p.goLabelsPending, pid)
			p.goLabelsLock.Unlock()

			if !p.hasEntry(pid) {
				p.countSpanCtx(spanCtxStepGoLabels, spanCtxNoProcessEntry)
				continue
			}
			err := p.resolveGoLabels(pid)
			p.reportSpanCtx(spanCtxStepGoLabels, pid, err)
		}
	}
}

// resolveGoLabels discovers the Go runtime offsets for pprof label reading
// and pushes them to the go_labels_procs BPF map.
func (p *EBPFResolver) resolveGoLabels(pid uint32) error {
	if p.goLabelsMap == nil {
		return fmt.Errorf("%w: go_labels_procs map not available", errSpanCtxMapError)
	}

	exePath := kernel.HostProc(strconv.FormatUint(uint64(pid), 10), "exe")

	layout, tlsErr, err := golabels.ResolveLayout(exePath)
	if err != nil {
		if errors.Is(err, golabels.ErrNotGo) {
			return fmt.Errorf("%w: %w", errSpanCtxGone, err)
		}
		return err
	}
	if tlsErr != nil {
		seclog.Debugf("Go labels TLS offset for pid %d: %s", pid, tlsErr)
	}

	if err := p.goLabelsMap.Put(pid, layout.Serialize()); err != nil {
		return fmt.Errorf("%w: %w", errSpanCtxMapError, err)
	}
	return nil
}

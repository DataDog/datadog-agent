// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package live

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/eventlog"
)

type captureProgress struct {
	mu                sync.Mutex
	duration          time.Duration
	phaseName, detail string
	logger            *eventlog.Logger
}

func newCaptureProgress(w io.Writer, duration time.Duration) *captureProgress {
	if w == nil {
		return nil
	}
	p := &captureProgress{duration: duration, logger: eventlog.New(w)}
	p.phase("initializing local authentication", "")
	return p
}

func (p *captureProgress) event(component, message string) {
	if p == nil {
		return
	}
	p.logger.Log(component, message)
}

func (p *captureProgress) phase(phase, detail string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.phaseName == phase && p.detail == detail {
		return
	}
	p.phaseName, p.detail = phase, detail
	if detail != "" {
		phase += "; " + detail
	}
	p.logger.Log("capture", phase)
}

func (p *captureProgress) armed(origin time.Time) {
	if p != nil {
		p.event("capture", fmt.Sprintf("recording started; duration=%s; ends=%s", p.duration, origin.Add(p.duration).Local().Format(time.RFC3339)))
	}
}

func (p *captureProgress) finish(directory string, err error) {
	if p == nil {
		return
	}
	message := "complete; bundle=" + directory
	if err != nil {
		message = "failed; no new bundle completed"
	}
	p.logger.Close("capture", message)
}

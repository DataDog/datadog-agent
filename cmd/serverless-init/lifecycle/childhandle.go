// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"os"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ChildHandle exposes the user process's liveness to the lifecycle server.
// /ready maps it directly: alive → 200, anything else → 503. The
// pre-spawn race is absorbed by the platform's /ready retry behavior.
type ChildHandle interface {
	IsAlive() bool
}

// Child is the production ChildHandle. mode.RunInit calls MarkAlive once
// cmd.Start succeeds and defers MarkDead so it fires whenever cmd.Wait
// returns (clean exit, signal, panic, runtime.Goexit). Safe for concurrent use.
type Child struct {
	alive *atomic.Bool
	proc  *atomic.Pointer[os.Process]
}

// NewChild returns a Child in the not-alive state.
func NewChild() *Child {
	return &Child{
		alive: atomic.NewBool(false),
		proc:  atomic.NewPointer[os.Process](nil),
	}
}

// StoreProcess records the OS process handle for use by SignalRun.
func (c *Child) StoreProcess(p *os.Process) { c.proc.Store(p) }

// IsAlive reports whether the child process is currently running.
func (c *Child) IsAlive() bool { return c.alive.Load() }

// MarkAlive records that the child process has started.
func (c *Child) MarkAlive() { c.alive.Store(true) }

// MarkDead records that the child process has exited.
func (c *Child) MarkDead() { c.alive.Store(false) }

// noopChild is the stub used in sidecar mode. IsAlive always reports false
// so /ready returns 503 — accurate for the no-managed-child scenario.
// Sidecar+MicroVM is documented as out of scope; the 503 surfaces that fact
// rather than papering over it with a static 200.
type noopChild struct{}

// NewNoopChildHandle returns a ChildHandle that always reports not-alive.
func NewNoopChildHandle() ChildHandle { return noopChild{} }
func (noopChild) IsAlive() bool       { return false }

// SignalRun delivers sig to the child. Returns nil if no process has been
// stored yet — /run may arrive before cmd.Start returns.
func (c *Child) SignalRun(sig os.Signal) error {
	p := c.proc.Load()
	if p == nil {
		return nil
	}
	return p.Signal(sig)
}

func (s *Server) sendRunSignal() {
	if !s.runSignalEnabled {
		return
	}
	if s.child == nil {
		return
	}
	if err := s.child.SignalRun(RunSignal); err != nil {
		log.Warnf("MicroVM lifecycle: /run signal (%v) to child failed: %v", RunSignal, err)
	}
}

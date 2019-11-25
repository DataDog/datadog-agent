// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package test

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/tinylib/msgp/msgp"
)

// ErrNotStarted is returned when attempting to operate an unstarted Runner.
var ErrNotStarted = errors.New("runner: not started")

// Runner can start an agent instance using a custom configuration, send payloads
// to it and act as a fake backend. Call Start first to initiate the fake backend,
// then RunAgent to start agent instances. Post may be used to send payloads to the
// agent and Out to receive its output.
type Runner struct {
	// Verbose will make the runner output more verbose, more specifically
	// around operations regarding the trace-agent process.
	Verbose bool

	// ChannelSize specifies the size of the payload buffer of the fake backend.
	// If reached, HTTP handlers will block until payloads are received from
	// the out channel. It defaults to 100.
	ChannelSize int

	agent   *agentRunner
	backend *fakeBackend
}

// Start initializes the runner and starts the fake backend.
func (s *Runner) Start() error {
	s.backend = newFakeBackend(s.ChannelSize)
	agent, err := newAgentRunner(s.backend.srv.Addr, s.Verbose)
	if err != nil {
		return err
	}
	s.agent = agent
	return s.backend.Start()
}

// Shutdown stops any running agent and shuts down the fake backend.
func (s *Runner) Shutdown(wait time.Duration) error {
	if s.agent == nil || s.backend == nil {
		return ErrNotStarted
	}
	s.agent.Kill()
	if err := s.backend.Shutdown(wait); err != nil {
		return err
	}
	s.agent = nil
	s.backend = nil
	return nil
}

// RunAgent starts an agent instance using the given YAML configuration.
func (s *Runner) RunAgent(conf []byte) error {
	if s.agent == nil {
		return ErrNotStarted
	}
	return s.agent.Run(conf)
}

// AgentLog returns up to 1MB of tail from the trace agent log.
func (s *Runner) AgentLog() string {
	if s.agent == nil {
		return ""
	}
	return s.agent.Log()
}

// KillAgent kills any agent that was started by this runner.
func (s *Runner) KillAgent() {
	if s.agent == nil {
		return
	}
	s.agent.Kill()
}

// Out returns a channel which will provide payloads received by the fake backend.
// They can be of type pb.TracePayload or agent.StatsPayload.
func (s *Runner) Out() <-chan interface{} {
	if s.backend == nil {
		closedCh := make(chan interface{})
		close(closedCh)
		return closedCh
	}
	return s.backend.Out()
}

// Post posts the given list of traces to the trace agent. Before posting, agent must
// be started. You can start an agent using RunAgent.
func (s *Runner) Post(traceList pb.Traces) error {
	if s.agent == nil {
		return ErrNotStarted
	}
	if s.agent.PID() == 0 {
		return errors.New("post: trace-agent not running")
	}

	var buf bytes.Buffer
	if err := msgp.Encode(&buf, traceList); err != nil {
		return err
	}
	addr := fmt.Sprintf("http://%s/v0.4/traces", s.agent.Addr())
	req, err := http.NewRequest("POST", addr, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("X-Datadog-Trace-Count", strconv.Itoa(len(traceList)))
	req.Header.Set("Content-Type", "application/msgpack")
	req.Header.Set("Content-Length", strconv.Itoa(buf.Len()))

	_, err = http.DefaultClient.Do(req)
	// TODO: check response
	return err
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetrycapture

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHardwareRequestRequiresAuthenticatedActiveSelectedSession(t *testing.T) {
	m := testManager(t, Software, HostSystemInfo)
	var calls atomic.Int32
	m.SetHostSystemInfoCollector(func(context.Context, Control) error {
		calls.Add(1)
		// Manager bookkeeping never remains locked during collection.
		_ = m.Status()
		return nil
	})
	handler, err := m.Handler(testAuth)
	if err != nil {
		t.Fatal(err)
	}
	if w := request(t, handler, http.MethodPost, "/host-system-info", validControlJSON, false); w.Code != http.StatusUnauthorized {
		t.Fatal("unauthenticated hardware collection")
	}
	if _, err := m.Prepare(PrepareRequest{Control: testControl, Streams: []Stream{Software}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RequestHostSystemInfo(context.Background(), testControl); err != ErrState {
		t.Fatal("prepared collection admitted")
	}
	if _, err := m.Activate(testControl); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RequestHostSystemInfo(context.Background(), testControl); err != ErrState {
		t.Fatal("unselected hardware collection admitted")
	}
	if _, err := m.Stop(context.Background(), testControl); err != nil {
		t.Fatal(err)
	}
	control := Control{ProtocolVersion: ProtocolVersion, SessionID: "hardware-session-000001"}
	if _, err := m.Prepare(PrepareRequest{Control: control, Streams: []Stream{HostSystemInfo}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Activate(control); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RequestHostSystemInfo(context.Background(), testControl); err != ErrSession {
		t.Fatal("obsolete session started collection")
	}
	for range 3 {
		if _, err := m.RequestHostSystemInfo(context.Background(), control); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("repeated control duplicated the hardware submission")
	}
	if _, err := m.Stop(context.Background(), control); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RequestHostSystemInfo(context.Background(), control); err != ErrState {
		t.Fatal("stopped capture triggered collection")
	}
}

func TestHardwareCancellationCannotReachANewSession(t *testing.T) {
	for _, operation := range []string{"stop", "caller-timeout", "shutdown"} {
		t.Run(operation, func(t *testing.T) {
			m := testManager(t, HostSystemInfo)
			started, release := make(chan struct{}), make(chan struct{})
			m.SetHostSystemInfoCollector(func(ctx context.Context, _ Control) error {
				close(started)
				<-release // A native OS query may not itself support cancellation.
				return ctx.Err()
			})
			arm(t, m, HostSystemInfo)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { _, err := m.RequestHostSystemInfo(ctx, testControl); result <- err }()
			<-started
			switch operation {
			case "stop":
				if _, err := m.Stop(context.Background(), testControl); err != nil {
					t.Fatal(err)
				}
			case "caller-timeout":
				cancel()
			case "shutdown":
				m.Close()
			}
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("cancelled hardware request succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("control remained blocked on cancelled native collection")
			}
			if _, err := m.RequestHostSystemInfo(context.Background(), testControl); err == nil {
				t.Fatal("timed-out request restarted collection")
			}
			newControl := Control{ProtocolVersion: ProtocolVersion, SessionID: "hardware-session-000002"}
			if _, err := m.Prepare(PrepareRequest{Control: newControl, Streams: []Stream{HostSystemInfo}}); err == nil {
				t.Fatal("new capture started while old collector could still send")
			}
			m.mu.Lock()
			done := m.current.hostSystemInfo.done
			m.mu.Unlock()
			close(release)
			<-done
			if operation != "shutdown" {
				if _, err := m.Prepare(PrepareRequest{Control: newControl, Streams: []Stream{HostSystemInfo}}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestHardwareFailureIsFixedAndFailsOnlyCapture(t *testing.T) {
	for _, panics := range []bool{false, true} {
		m := testManager(t, HostSystemInfo)
		m.SetHostSystemInfoCollector(func(context.Context, Control) error {
			if panics {
				panic("private-hardware-error")
			}
			return errors.New("private-hardware-error")
		})
		arm(t, m, HostSystemInfo)
		handler, err := m.Handler(testAuth)
		if err != nil {
			t.Fatal(err)
		}
		w := request(t, handler, http.MethodPost, "/host-system-info", validControlJSON, true)
		if w.Code != http.StatusConflict || strings.Contains(w.Body.String(), "private-hardware-error") || m.Enabled() || m.Status().State != Failed {
			t.Fatal("unsafe hardware failure response")
		}
	}
}

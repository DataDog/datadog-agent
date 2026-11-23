// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcx509clientimpl

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	inventorychecks "github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
)

// fakeInventory records every Set call.
type fakeInventory struct {
	inventorychecks.Component

	mu    sync.Mutex
	calls []setCall
}

type setCall struct {
	instance, key string
	value         interface{}
}

func (f *fakeInventory) Set(instanceID, key string, value interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, setCall{instanceID, key, value})
}

// latest returns the last value set per instance for the tunnel key.
func (f *fakeInventory) latest() map[string]interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]interface{}{}
	for _, c := range f.calls {
		if c.key == tunnelConnectionIDKey {
			out[c.instance] = c.value
		}
	}
	return out
}

func (f *fakeInventory) snapshot() []setCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]setCall(nil), f.calls...)
}

func (f *fakeInventory) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// tunnelCheck is a loaded check instance. supports controls the
// RemoteQuerier answer; a plain stub check does not implement the interface.
type tunnelCheck struct {
	stub.StubCheck
	id       checkid.ID
	supports bool
}

func (c *tunnelCheck) ID() checkid.ID              { return c.id }
func (c *tunnelCheck) String() string              { return "postgres" }
func (c *tunnelCheck) SupportsRemoteQueries() bool { return c.supports }
func (c *tunnelCheck) RemoteQuery(context.Context, string) (string, error) {
	return "", nil
}

// coreCheck is a Go core check without remote query support.
type coreCheck struct {
	stub.StubCheck
	id checkid.ID
}

func (c *coreCheck) ID() checkid.ID { return c.id }

type loadedChecks struct {
	mu     sync.Mutex
	checks []check.Check
}

func (l *loadedChecks) add(c ...check.Check) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.checks = append(l.checks, c...)
}

func (l *loadedChecks) get() []check.Check {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]check.Check(nil), l.checks...)
}

type publishHarness struct {
	*reporterHarness
	inventory *fakeInventory
	loaded    *loadedChecks
}

func newPublishHarness(t *testing.T, logConnection bool) *publishHarness {
	t.Helper()
	p := &publishHarness{reporterHarness: newHarness(t, nil, nil), inventory: &fakeInventory{}, loaded: &loadedChecks{}}
	p.loaded.add(
		&tunnelCheck{id: "postgres:a", supports: true},
		&tunnelCheck{id: "postgres:unsupported", supports: false},
		&coreCheck{id: "cpu:1"},
	)
	p.r.logConnection = logConnection
	p.r.publisher = newTunnelMetadataPublisher(p.inventory, p.loaded.get)
	require.NotNil(t, p.r.publisher)
	require.True(t, p.r.install())
	return p
}

func (p *publishHarness) requireLatest(t *testing.T, want map[string]interface{}) {
	t.Helper()
	require.Eventually(t, func() bool {
		got := p.inventory.latest()
		if len(got) != len(want) {
			return false
		}
		for k, v := range want {
			if got[k] != v {
				return false
			}
		}
		return true
	}, 5*time.Second, 5*time.Millisecond, "inventory: %v", p.inventory.latest())
}

// settle proves that the ticker goroutine handled everything signalled before:
// an unbuffered tick is only received between iterations of the loop.
func (p *publishHarness) settle() {
	p.pubTicker.c <- time.Time{}
	p.pubTicker.c <- time.Time{}
}

func TestPublishesNothingWithoutAnExecuteHandler(t *testing.T) {
	p := newPublishHarness(t, true)
	p.write(t, handshakeLine(testID1))
	require.Contains(t, p.logger.next(t), testID1) // the connection is captured and logged...
	p.settle()
	p.settle()
	require.Zero(t, p.inventory.count(), "...but not published without a registered handler")
	p.shutdown(t)
}

func TestPublishesToEligibleInstancesOnly(t *testing.T) {
	p := newPublishHarness(t, true)
	p.r.markExecuteHandlerRegistered()
	// No handshake yet: nothing to publish.
	p.settle()
	require.Zero(t, p.inventory.count())

	p.write(t, handshakeLine(strings.ToUpper(testID1)))
	p.requireLatest(t, map[string]interface{}{"postgres:a": testID1})
	p.settle()
	p.requireLatest(t, map[string]interface{}{"postgres:a": testID1})

	for _, c := range p.inventory.snapshot() {
		require.Equal(t, tunnelConnectionIDKey, c.key)
		require.Equal(t, testID1, c.value)
		require.NotEqual(t, "postgres:unsupported", c.instance, "a check that does not serve remote queries must not be marked")
		require.NotEqual(t, "cpu:1", c.instance, "a Go core check without RemoteQuerier must not be marked")
	}
	p.shutdown(t)
}

func TestPublishesAfterTheHandlerIsRegisteredLate(t *testing.T) {
	p := newPublishHarness(t, true)
	p.write(t, handshakeLine(testID1))
	require.Contains(t, p.logger.next(t), "execute_handler=false")
	p.settle()
	require.Zero(t, p.inventory.count())

	p.r.markExecuteHandlerRegistered()
	p.requireLatest(t, map[string]interface{}{"postgres:a": testID1})
	p.shutdown(t)
}

func TestReconnectPublishesTheNewID(t *testing.T) {
	p := newPublishHarness(t, true)
	p.r.markExecuteHandlerRegistered()
	p.write(t, handshakeLine(testID1))
	p.requireLatest(t, map[string]interface{}{"postgres:a": testID1})

	p.write(t, handshakeLine(testID2))
	p.requireLatest(t, map[string]interface{}{"postgres:a": testID2})
	// A later tick never brings the old ID back.
	p.settle()
	p.requireLatest(t, map[string]interface{}{"postgres:a": testID2})
	p.shutdown(t)
}

func TestLaterScheduledInstanceGetsTheKeyOnTheNextTick(t *testing.T) {
	p := newPublishHarness(t, true)
	p.r.markExecuteHandlerRegistered()
	p.write(t, handshakeLine(testID1))
	p.requireLatest(t, map[string]interface{}{"postgres:a": testID1})

	p.loaded.add(
		&tunnelCheck{id: "postgres:b", supports: true},
		&coreCheck{id: "disk:1"},
		&tunnelCheck{id: "postgres:late-unsupported"},
	)
	p.pubTicker.c <- time.Time{}
	p.requireLatest(t, map[string]interface{}{"postgres:a": testID1, "postgres:b": testID1})
	p.shutdown(t)
}

func TestPublishingIsIndependentOfTheLogLine(t *testing.T) {
	p := newPublishHarness(t, false)
	require.Equal(t, []time.Duration{tunnelMetadataInterval}, p.intervals, "no log ticker without the log line")
	p.r.markExecuteHandlerRegistered()
	p.write(t, handshakeLine(testID1))
	p.requireLatest(t, map[string]interface{}{"postgres:a": testID1})
	p.settle()
	p.shutdown(t)
	p.logger.requireNone(t)
}

func TestPublishesNothingAfterClose(t *testing.T) {
	p := newPublishHarness(t, true)
	p.r.markExecuteHandlerRegistered()
	p.write(t, handshakeLine(testID1))
	p.requireLatest(t, map[string]interface{}{"postgres:a": testID1})
	p.shutdown(t)

	before := p.inventory.count()
	p.loaded.add(&tunnelCheck{id: "postgres:b", supports: true})
	p.r.publish()
	require.Equal(t, before, p.inventory.count())
}

func TestNoPublisherWithoutInventoryOrChecks(t *testing.T) {
	require.Nil(t, newTunnelMetadataPublisher(nil, func() []check.Check { return nil }))
	require.Nil(t, newTunnelMetadataPublisher(&fakeInventory{}, nil))

	// A reporter without a publisher only logs.
	h := newHarness(t, nil, nil)
	require.True(t, h.r.install())
	require.Equal(t, []time.Duration{connectionReportInterval}, h.intervals)
	h.r.markExecuteHandlerRegistered()
	h.write(t, handshakeLine(testID1))
	require.Contains(t, h.logger.next(t), testID1)
	h.r.publish() // inert
	h.shutdown(t)
}

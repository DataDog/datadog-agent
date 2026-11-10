// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package storeimpl

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

func (h *healthPlatformImpl) queueLen() int {
	h.ingestMu.Lock()
	defer h.ingestMu.Unlock()
	return len(h.ingestQueue)
}

func TestIngestOverflow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		for i := 0; i < ingestChanBuf+100; i++ {
			require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: strconv.Itoa(i), IssueName: "test"}))
		}
		assert.Equal(t, ingestChanBuf, h.queueLen())
		assert.Equal(t, 100.0, h.metrics.issuesDropped.WithValues("report").Get())
		assert.Equal(t, float64(ingestChanBuf), h.metrics.ingestQueueLen.WithValues().Get())
		startTestStore(t, h)
		synctest.Wait()
		h.sync()
		count, _ := h.GetAllIssues()
		assert.Equal(t, ingestChanBuf, count)
		for i := 0; i < 100; i++ {
			assert.Nil(t, h.GetIssue(strconv.Itoa(i)))
		}
		for i := 100; i < ingestChanBuf+100; i++ {
			assert.NotNil(t, h.GetIssue(strconv.Itoa(i)))
		}
		assert.Equal(t, float64(ingestChanBuf), h.metrics.issuesCounter.WithValues("test", "ISSUE_SEVERITY_UNSPECIFIED").Get())
	})
}

func TestIngestOverflowPreservesResolutions(t *testing.T) {
	for _, resolveAll := range []bool{false, true} {
		t.Run(fmt.Sprintf("resolve_all=%t", resolveAll), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newUnstartedTestStore(t)
				p := &controlledPersistence{}
				h.persistence = p
				for i := 0; i < ingestChanBuf; i++ {
					require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: strconv.Itoa(i), IssueName: "test"}))
				}
				id := strconv.Itoa(ingestChanBuf - 1)
				if resolveAll {
					h.ResolveAllIssues()
				} else {
					h.ResolveIssue(id)
				}
				assert.Equal(t, ingestChanBuf, h.queueLen())
				assert.Zero(t, h.metrics.issuesDropped.WithValues("resolve").Get())
				assert.Zero(t, h.metrics.issuesDropped.WithValues("resolve_all").Get())
				assert.Equal(t, 1.0, h.metrics.issuesDropped.WithValues("report").Get())
				startTestStore(t, h)
				synctest.Wait()
				h.sync()
				assert.Nil(t, h.GetIssue(id))
				assert.Nil(t, h.GetIssue("0"))
				stopTestStore(t, h)
				state := p.snapshot()
				require.NotNil(t, state)
				require.Contains(t, state.Issues, id)
				assert.Equal(t, IssueStateResolved, state.Issues[id].State)
				assert.NotContains(t, state.Issues, "0")
			})
		})
	}
}

func TestIngestOverflowWithoutReportsPreservesBarriers(t *testing.T) {
	h := newUnstartedTestStore(t)
	for i := 0; i < ingestChanBuf; i++ {
		h.ResolveIssue(strconv.Itoa(i))
	}
	h.ResolveAllIssues()
	done := make(chan struct{})
	h.enqueue(ingestCmd{kind: cmdSync, done: done})
	require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: "dropped", IssueName: "test"}))
	assert.Equal(t, ingestChanBuf+2, h.queueLen())
	assert.Zero(t, h.metrics.issuesDropped.WithValues("resolve").Get())
	assert.Zero(t, h.metrics.issuesDropped.WithValues("resolve_all").Get())
	assert.Zero(t, h.metrics.issuesDropped.WithValues("sync").Get())
	assert.Equal(t, 1.0, h.metrics.issuesDropped.WithValues("report").Get())
	startTestStore(t, h)
	<-done
	h.sync()
	assert.Nil(t, h.GetIssue("dropped"))
}

func TestIngestVisibilityAndQueueMetric(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: "id", IssueName: "test"}))
		assert.Nil(t, h.GetIssue("id"))
		startTestStore(t, h)
		h.sync()
		synctest.Wait()
		require.NotNil(t, h.GetIssue("id"))
		assert.Equal(t, 1.0, h.metrics.issuesCounter.WithValues("test", "ISSUE_SEVERITY_UNSPECIFIED").Get())
		assert.Zero(t, h.metrics.ingestQueueLen.WithValues().Get())
	})
}

func TestIngestOrdering(t *testing.T) {
	for _, resolveAll := range []bool{false, true} {
		t.Run(fmt.Sprintf("resolve_all=%t", resolveAll), func(t *testing.T) {
			h := newUnstartedTestStore(t)
			obs := newTestObserver(1)
			h.RegisterIssuesObserver(obs)
			resolve := func() {
				if resolveAll {
					h.ResolveAllIssues()
				} else {
					h.ResolveIssue("id")
				}
			}
			require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: "id", IssueName: "test", Title: "before"}))
			resolve()
			startTestStore(t, h)
			h.sync()
			assert.Nil(t, h.GetIssue("id"))
			require.Len(t, obs.ResolvedCh, 1)
			assert.Equal(t, IssueStateResolved, (<-obs.ResolvedCh).PersistedIssue.State)

			require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: "id", IssueName: "test", Title: "before"}))
			resolve()
			require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: "id", IssueName: "test", Title: "after"}))
			h.sync()
			issue := h.GetIssue("id")
			require.NotNil(t, issue)
			assert.Equal(t, "after", issue.Title)
			assert.Equal(t, IssueStateActive, issue.PersistedIssue.State)
			assert.Empty(t, issue.PersistedIssue.GetResolvedAt())
			require.Len(t, obs.ResolvedCh, 1)
			assert.Equal(t, IssueStateResolved, (<-obs.ResolvedCh).PersistedIssue.State)
		})
	}
}

func TestIngestClonesCallerIssue(t *testing.T) {
	h := newUnstartedTestStore(t)
	issue := &healthplatform.Issue{
		Id: "id", IssueName: "test", Title: "original", Tags: []string{"original"},
		Extra:       &structpb.Struct{Fields: map[string]*structpb.Value{"key": structpb.NewStringValue("original")}},
		Remediation: &healthplatform.Remediation{Summary: "original"},
	}
	require.NoError(t, h.ReportIssue(issue))
	issue.Title = "changed"
	issue.Tags[0] = "changed"
	issue.Extra.Fields["key"] = structpb.NewStringValue("changed")
	issue.Remediation.Summary = "changed"
	startTestStore(t, h)
	h.sync()
	got := h.GetIssue("id")
	require.NotNil(t, got)
	assert.Equal(t, "original", got.Title)
	assert.Equal(t, []string{"original"}, got.Tags)
	assert.Equal(t, "original", got.Extra.Fields["key"].GetStringValue())
	assert.Equal(t, "original", got.Remediation.Summary)
	assert.Empty(t, issue.DetectedAt)
	assert.Nil(t, issue.PersistedIssue)
}

type controlledPersistence struct {
	memPersistence
	entered chan struct{}
	release chan struct{}
	saves   atomic.Int32
	err     error
}

func (p *controlledPersistence) save(state *PersistedState) error {
	p.saves.Add(1)
	if p.entered != nil {
		p.entered <- struct{}{}
		<-p.release
	}
	if p.err != nil {
		return p.err
	}
	return p.memPersistence.save(state)
}

func (p *controlledPersistence) snapshot() *PersistedState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func TestIngestDoesNotBlockOnPersistence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		p := &controlledPersistence{entered: make(chan struct{}, 1), release: make(chan struct{})}
		h.persistence = p
		startTestStore(t, h)
		t.Cleanup(func() { close(p.release) })
		require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: "first", IssueName: "test"}))
		h.sync()
		time.Sleep(flushInterval)
		<-p.entered
		require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: "second", IssueName: "test"}))
		h.ResolveIssue("first")
		h.ResolveAllIssues()
		require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: "third", IssueName: "test"}))
		assert.Equal(t, 4, h.queueLen())
		assert.Nil(t, h.GetIssue("second"))
		p.release <- struct{}{}
		h.sync()
		assert.Nil(t, h.GetIssue("first"))
		assert.Nil(t, h.GetIssue("second"))
		require.NotNil(t, h.GetIssue("third"))
	})
}

func TestIngestCoalescesPersistence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		p := &controlledPersistence{}
		h.persistence = p
		startTestStore(t, h)
		for i := 0; i < 100; i++ {
			require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: strconv.Itoa(i), IssueName: "test"}))
		}
		h.sync()
		synctest.Wait()
		assert.Zero(t, p.saves.Load())
		time.Sleep(flushInterval)
		synctest.Wait()
		assert.Equal(t, int32(1), p.saves.Load())
		state := p.snapshot()
		require.NotNil(t, state)
		assert.Len(t, state.Issues, 100)
		time.Sleep(2 * flushInterval)
		synctest.Wait()
		assert.Equal(t, int32(1), p.saves.Load())
		h.ResolveIssue("missing")
		h.sync()
		time.Sleep(flushInterval)
		synctest.Wait()
		assert.Equal(t, int32(1), p.saves.Load())
		stopTestStore(t, h)
		assert.Equal(t, int32(1), p.saves.Load())
	})
}

func TestIngestDrainServicesPersistenceTicks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		p := &controlledPersistence{}
		h.persistence = p
		require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: "first", IssueName: "test"}))
		h.drain(false, nil)
		require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: "second", IssueName: "test"}))
		require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: "third", IssueName: "test"}))
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()
		time.Sleep(flushInterval)
		h.drain(true, ticker.C)
		assert.Equal(t, int32(1), p.saves.Load())
		state := p.snapshot()
		require.NotNil(t, state)
		assert.Contains(t, state.Issues, "first")
		assert.NotContains(t, state.Issues, "second")
		assert.NotContains(t, state.Issues, "third")
		count, _ := h.GetAllIssues()
		assert.Equal(t, 3, count)
	})
}

func TestIngestShutdownDrainsAndFlushes(t *testing.T) {
	h := newUnstartedTestStore(t)
	p := &controlledPersistence{}
	h.persistence = p
	for i := 0; i < 100; i++ {
		require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: strconv.Itoa(i), IssueName: "test"}))
	}
	h.ResolveIssue("0")
	startTestStore(t, h)
	stopTestStore(t, h)
	assert.Zero(t, h.queueLen())
	state := p.snapshot()
	require.NotNil(t, state)
	assert.Len(t, state.Issues, 100)
	assert.Equal(t, IssueStateResolved, state.Issues["0"].State)
	assert.Nil(t, h.GetIssue("0"))
	count, _ := h.GetAllIssues()
	assert.Equal(t, 99, count)
	for i := 0; i < ingestChanBuf+1; i++ {
		require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: "late", IssueName: "test"}))
	}
	assert.Equal(t, 1.0, h.metrics.issuesDropped.WithValues("report").Get())
	assert.Nil(t, h.GetIssue("late"))
}

func TestIngestShutdownRespectsContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		p := &controlledPersistence{entered: make(chan struct{}, 1), release: make(chan struct{})}
		h.persistence = p
		require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: "id", IssueName: "test"}))
		require.NoError(t, h.start(context.Background()))
		t.Cleanup(func() {
			close(p.release)
			select {
			case <-h.stopCh:
			default:
				require.NoError(t, h.stop(context.Background()))
			}
			<-h.doneCh
		})
		<-p.entered

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		stopped := make(chan error, 1)
		go func() { stopped <- h.stop(ctx) }()
		select {
		case err := <-stopped:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("stop did not return within one second")
		}
		assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
		select {
		case <-h.doneCh:
			t.Fatal("ingest worker exited while persistence was blocked")
		default:
		}
	})
}

func TestIngestPersistenceFailureDoesNotBlockFurtherCommands(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		p := &controlledPersistence{err: assert.AnError}
		h.persistence = p
		startTestStore(t, h)
		require.NoError(t, h.ReportIssue(&healthplatform.Issue{Id: "id", IssueName: "test"}))
		h.sync()
		time.Sleep(flushInterval)
		synctest.Wait()
		assert.Equal(t, int32(1), p.saves.Load())
		time.Sleep(flushInterval)
		synctest.Wait()
		assert.Equal(t, int32(2), p.saves.Load())
		h.ResolveIssue("id")
		h.sync()
		assert.Nil(t, h.GetIssue("id"))
		stopTestStore(t, h)
		assert.Equal(t, int32(3), p.saves.Load())
	})
}

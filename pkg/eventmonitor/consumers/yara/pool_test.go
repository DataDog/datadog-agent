// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// poolTestDeduper counts the hash releases
type poolTestDeduper struct {
	*StandInDeduper
	mu       sync.Mutex
	released map[[32]byte]int
}

func newPoolTestDeduper() *poolTestDeduper {
	return &poolTestDeduper{StandInDeduper: NewStandInDeduper(), released: make(map[[32]byte]int)}
}

func (d *poolTestDeduper) ReleaseHash(sum [32]byte) {
	d.mu.Lock()
	d.released[sum]++
	d.mu.Unlock()
	d.StandInDeduper.ReleaseHash(sum)
}

func (d *poolTestDeduper) releasedCount(sum [32]byte) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.released[sum]
}

func (d *poolTestDeduper) totalReleased() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, c := range d.released {
		n += c
	}
	return n
}

// poolTestJobs builds jobs and checks that each Done is called exactly once
type poolTestJobs struct {
	t     *testing.T
	mu    sync.Mutex
	dones []*atomic.Int32
	wg    sync.WaitGroup
}

func newPoolTestJobs(t *testing.T) *poolTestJobs {
	return &poolTestJobs{t: t}
}

func (j *poolTestJobs) job(data string) ScanJob {
	j.mu.Lock()
	idx := len(j.dones)
	count := &atomic.Int32{}
	j.dones = append(j.dones, count)
	j.mu.Unlock()
	j.wg.Add(1)

	return ScanJob{
		File: ExecFile{Path: fmt.Sprintf("/bin/job-%d", idx), PID: uint32(idx)},
		Sum:  sha256.Sum256([]byte(fmt.Sprintf("%d:%s", idx, data))),
		Data: []byte(data),
		Done: func() {
			if count.Add(1) == 1 {
				j.wg.Done()
			}
		},
	}
}

// waitAllDone waits for every Done, then checks none was called twice
func (j *poolTestJobs) waitAllDone() {
	j.t.Helper()
	ch := make(chan struct{})
	go func() {
		j.wg.Wait()
		close(ch)
	}()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		j.t.Fatal("timeout waiting for every job's Done")
	}
	j.assertDoneOnce()
}

func (j *poolTestJobs) assertDoneOnce() {
	j.t.Helper()
	j.mu.Lock()
	defer j.mu.Unlock()
	for i, c := range j.dones {
		assert.EqualValues(j.t, 1, c.Load(), "Done of job %d", i)
	}
}

// blockingScanner blocks every scan until unblock is closed or ctx is done
type blockingScanner struct {
	started chan struct{}
	unblock chan struct{}
}

func newBlockingScanner() *blockingScanner {
	return &blockingScanner{started: make(chan struct{}, 1024), unblock: make(chan struct{})}
}

func (s *blockingScanner) Scan(ctx context.Context, _ []byte) ([]Match, error) {
	s.started <- struct{}{}
	select {
	case <-s.unblock:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *blockingScanner) RulesVersion() string { return "blocking" }

func (s *blockingScanner) waitStarted(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-s.started:
		case <-time.After(10 * time.Second):
			t.Fatalf("timeout waiting for scan %d to start", i)
		}
	}
}

type panicScanner struct{}

func (panicScanner) Scan(context.Context, []byte) ([]Match, error) { panic("scanner bug") }
func (panicScanner) RulesVersion() string                          { return "panic" }

func newTestPool(t *testing.T, cfg PoolConfig, scanner Scanner) (*Pool, *poolTestDeduper, *recordingReporter, *Stats) {
	t.Helper()
	deduper := newPoolTestDeduper()
	reporter := &recordingReporter{}
	stats := &Stats{}
	pool, err := NewPool(cfg, scanner, deduper, reporter, stats)
	require.NoError(t, err)
	return pool, deduper, reporter, stats
}

func reportsOf(r *recordingReporter) []report {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]report(nil), r.reports...)
}

func TestNewPoolValidation(t *testing.T) {
	s := NewMarkerScanner("m", "r")
	d := NewStandInDeduper()
	r := &recordingReporter{}

	for _, cfg := range []PoolConfig{
		{Workers: 0, QueueSize: 1},
		{Workers: 1, QueueSize: -1},
		{Workers: 1, ScanTimeout: -time.Second},
		{Workers: 1, MaxFileSize: -1},
		{Workers: 1, MaxBufferedBytes: -1},
	} {
		_, err := NewPool(cfg, s, d, r, nil)
		assert.Error(t, err, "%+v", cfg)
	}

	_, err := NewPool(PoolConfig{Workers: 1}, nil, d, r, nil)
	assert.Error(t, err)

	pool, err := NewPool(PoolConfig{Workers: 1}, s, d, r, nil)
	require.NoError(t, err)
	assert.NotNil(t, pool.stats, "nil stats must be replaced")
}

func TestPoolByteBudgetDefault(t *testing.T) {
	assert.EqualValues(t, 0, PoolConfig{Workers: 2}.byteBudget())
	assert.EqualValues(t, 2*64, PoolConfig{Workers: 2, MaxFileSize: 64}.byteBudget())
	assert.EqualValues(t, 10, PoolConfig{Workers: 2, MaxFileSize: 64, MaxBufferedBytes: 10}.byteBudget())
}

func TestPoolScansAndReports(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	pool, deduper, reporter, stats := newTestPool(t, PoolConfig{Workers: 3, QueueSize: 32, ScanTimeout: time.Minute},
		NewMarkerScanner("EVIL", "evil_rule"))
	var observed atomic.Int32
	stats.ObserveScanDuration = func(time.Duration) { observed.Add(1) }
	pool.Start()

	jobs := newPoolTestJobs(t)
	for i := 0; i < 20; i++ {
		data := "clean"
		if i%4 == 0 {
			data = "has EVIL inside"
		}
		require.True(t, pool.Submit(jobs.job(data)))
	}
	jobs.waitAllDone()
	pool.Stop()

	assert.EqualValues(t, 20, stats.Scans.Load())
	assert.EqualValues(t, 5, stats.Matches.Load())
	assert.EqualValues(t, 0, stats.ScanErrors.Load())
	assert.EqualValues(t, 0, stats.ScanTimeouts.Load())
	assert.EqualValues(t, 0, stats.QueueDrops.Load())
	assert.EqualValues(t, 20, observed.Load())
	assert.Equal(t, 0, deduper.totalReleased(), "successful scans must keep their hash")
	assert.EqualValues(t, 0, pool.bufferedBytes())
	assert.Equal(t, 0, pool.QueueDepth())

	reports := reportsOf(reporter)
	require.Len(t, reports, 20)
	matched := 0
	for _, r := range reports {
		assert.NoError(t, r.err)
		if len(r.matches) > 0 {
			matched++
			assert.Equal(t, "evil_rule", r.matches[0].Rule)
		}
	}
	assert.Equal(t, 5, matched)
}

func TestPoolFloodDropsWithoutBlocking(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	const workers, queueSize, flood = 2, 4, 1000
	scanner := newBlockingScanner()
	pool, deduper, reporter, stats := newTestPool(t, PoolConfig{Workers: workers, QueueSize: queueSize}, scanner)
	pool.Start()

	jobs := newPoolTestJobs(t)
	// occupy every worker
	for i := 0; i < workers; i++ {
		require.True(t, pool.Submit(jobs.job("busy")))
	}
	scanner.waitStarted(t, workers)

	var dropped []ScanJob
	start := time.Now()
	for i := 0; i < flood; i++ {
		job := jobs.job("flood")
		if !pool.Submit(job) {
			dropped = append(dropped, job)
		}
	}
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 2*time.Second, "Submit must not block while the workers are stuck")

	assert.Len(t, dropped, flood-queueSize)
	assert.EqualValues(t, flood-queueSize, stats.QueueDrops.Load())
	assert.Equal(t, queueSize, pool.QueueDepth())
	for _, job := range dropped {
		assert.Equal(t, 1, deduper.releasedCount(job.Sum), "dropped job's hash must be released")
	}
	assert.Equal(t, flood-queueSize, deduper.totalReleased())

	close(scanner.unblock)
	jobs.waitAllDone()
	pool.Stop()

	assert.EqualValues(t, workers+queueSize, stats.Scans.Load())
	assert.Len(t, reportsOf(reporter), workers+queueSize)
	assert.EqualValues(t, 0, pool.bufferedBytes())
}

func TestPoolByteBudget(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	scanner := newBlockingScanner()
	pool, deduper, _, stats := newTestPool(t, PoolConfig{Workers: 1, QueueSize: 10, MaxBufferedBytes: 100}, scanner)
	pool.Start()
	jobs := newPoolTestJobs(t)
	data := func(n int) string { return string(make([]byte, n)) }

	require.True(t, pool.Submit(jobs.job(data(60))), "in flight")
	scanner.waitStarted(t, 1)

	big := jobs.job(data(50))
	assert.False(t, pool.Submit(big), "60+50 is over the budget")
	assert.Equal(t, 1, deduper.releasedCount(big.Sum))
	assert.True(t, pool.Submit(jobs.job(data(40))), "60+40 fits exactly")
	assert.False(t, pool.Submit(jobs.job(data(1))), "budget exhausted even though the queue has room")
	assert.False(t, pool.Submit(jobs.job(data(101))), "larger than the whole budget")
	assert.EqualValues(t, 100, pool.bufferedBytes())
	assert.EqualValues(t, 3, stats.QueueDrops.Load())

	// the budget is freed once the jobs are done
	close(scanner.unblock)
	require.Eventually(t, func() bool { return pool.bufferedBytes() == 0 }, 10*time.Second, time.Millisecond)
	assert.True(t, pool.Submit(jobs.job(data(100))))

	jobs.waitAllDone()
	pool.Stop()
	assert.EqualValues(t, 3, stats.Scans.Load())
	assert.EqualValues(t, 0, pool.bufferedBytes())
}

func TestPoolTimeout(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	pool, deduper, reporter, stats := newTestPool(t, PoolConfig{Workers: 1, QueueSize: 1, ScanTimeout: 20 * time.Millisecond},
		newBlockingScanner())
	var observed atomic.Int32
	stats.ObserveScanDuration = func(time.Duration) { observed.Add(1) }
	pool.Start()

	jobs := newPoolTestJobs(t)
	job := jobs.job("slow")
	require.True(t, pool.Submit(job))
	jobs.waitAllDone()
	pool.Stop()

	assert.EqualValues(t, 1, stats.ScanTimeouts.Load())
	assert.EqualValues(t, 0, stats.ScanErrors.Load())
	assert.EqualValues(t, 0, stats.Scans.Load())
	assert.EqualValues(t, 1, observed.Load())
	assert.Equal(t, 1, deduper.releasedCount(job.Sum), "a timed out scan must release its hash")

	reports := reportsOf(reporter)
	require.Len(t, reports, 1)
	assert.ErrorIs(t, reports[0].err, context.DeadlineExceeded)
	assert.Equal(t, job.File, reports[0].file)
	assert.Equal(t, job.Sum, reports[0].sum)
}

func TestPoolScanErrors(t *testing.T) {
	for name, scanner := range map[string]Scanner{
		"error": failingScanner{},
		"panic": panicScanner{},
	} {
		t.Run(name, func(t *testing.T) {
			defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

			pool, deduper, reporter, stats := newTestPool(t, PoolConfig{Workers: 2, QueueSize: 4}, scanner)
			pool.Start()

			jobs := newPoolTestJobs(t)
			job := jobs.job("data")
			require.True(t, pool.Submit(job))
			jobs.waitAllDone()
			pool.Stop()

			assert.EqualValues(t, 1, stats.ScanErrors.Load())
			assert.EqualValues(t, 0, stats.ScanTimeouts.Load())
			assert.EqualValues(t, 0, stats.Scans.Load())
			assert.Equal(t, 1, deduper.releasedCount(job.Sum), "a failed scan must release its hash")

			reports := reportsOf(reporter)
			require.Len(t, reports, 1)
			assert.Error(t, reports[0].err)
			assert.Nil(t, reports[0].matches)
		})
	}
}

func TestPoolStop(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	const workers, queueSize = 2, 5
	scanner := newBlockingScanner()
	pool, deduper, reporter, stats := newTestPool(t, PoolConfig{Workers: workers, QueueSize: queueSize}, scanner)
	pool.Start()

	jobs := newPoolTestJobs(t)
	var accepted []ScanJob
	for i := 0; i < workers; i++ {
		job := jobs.job("in flight")
		require.True(t, pool.Submit(job))
		accepted = append(accepted, job)
	}
	scanner.waitStarted(t, workers)
	for i := 0; i < queueSize; i++ {
		job := jobs.job("queued")
		require.True(t, pool.Submit(job))
		accepted = append(accepted, job)
	}

	stopped := make(chan struct{})
	go func() {
		pool.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("Stop hung")
	}

	// every accepted job is done, with its hash released, and nothing was reported
	jobs.assertDoneOnce()
	for _, job := range accepted {
		assert.Equal(t, 1, deduper.releasedCount(job.Sum))
	}
	assert.Empty(t, reportsOf(reporter))
	assert.EqualValues(t, 0, stats.ScanErrors.Load())
	assert.EqualValues(t, 0, stats.ScanTimeouts.Load())
	assert.EqualValues(t, 0, pool.bufferedBytes())
	assert.Equal(t, 0, pool.QueueDepth())

	// Submit after Stop drops
	late := jobs.job("late")
	assert.False(t, pool.Submit(late))
	assert.Equal(t, 1, deduper.releasedCount(late.Sum))
	jobs.waitAllDone()

	// Stop and Start after Stop are no-ops
	pool.Stop()
	pool.Start()
	assert.False(t, pool.Submit(jobs.job("after restart")))
	jobs.waitAllDone()
}

func TestPoolStopWithoutStart(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	pool, deduper, _, _ := newTestPool(t, PoolConfig{Workers: 1, QueueSize: 2}, NewMarkerScanner("m", "r"))
	jobs := newPoolTestJobs(t)
	job := jobs.job("queued before start")
	require.True(t, pool.Submit(job), "jobs can wait in the queue before Start")
	assert.Equal(t, 1, pool.QueueDepth())

	pool.Stop()
	jobs.waitAllDone()
	assert.Equal(t, 1, deduper.releasedCount(job.Sum))
}

func TestPoolNilDone(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	pool, _, reporter, _ := newTestPool(t, PoolConfig{Workers: 1, QueueSize: 0}, NewMarkerScanner("m", "r"))
	// queue size 0: nothing is accepted before a worker is idle
	assert.False(t, pool.Submit(ScanJob{Data: []byte("m")}))

	pool.Start()
	require.Eventually(t, func() bool { return pool.Submit(ScanJob{Data: []byte("m")}) }, 10*time.Second, time.Millisecond)
	require.Eventually(t, func() bool { return len(reportsOf(reporter)) == 1 }, 10*time.Second, time.Millisecond)
	pool.Stop()
}

// TestPoolConcurrentSubmitAndStop checks, under the race detector, that every Done is called
// exactly once when Submit races with Stop
func TestPoolConcurrentSubmitAndStop(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	pool, _, _, stats := newTestPool(t, PoolConfig{Workers: 4, QueueSize: 16, MaxBufferedBytes: 64, ScanTimeout: time.Second},
		NewMarkerScanner("EVIL", "evil_rule"))
	pool.Start()

	jobs := newPoolTestJobs(t)
	const submitters, perSubmitter = 8, 200
	var accepted atomic.Int64
	var wg sync.WaitGroup
	wg.Add(submitters)
	for i := 0; i < submitters; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perSubmitter; j++ {
				if pool.Submit(jobs.job("some EVIL data")) {
					accepted.Add(1)
				}
			}
		}()
	}
	time.Sleep(time.Millisecond)
	pool.Stop()
	wg.Wait()
	jobs.waitAllDone()

	assert.EqualValues(t, submitters*perSubmitter, accepted.Load()+stats.QueueDrops.Load())
	assert.EqualValues(t, 0, pool.bufferedBytes())
}

func TestPoolReportErrorNotLeakedAcrossJobs(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// the first scan fails, the second succeeds: only the first releases its hash
	var calls atomic.Int32
	scanner := scanFunc(func(context.Context, []byte) ([]Match, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("first fails")
		}
		return nil, nil
	})
	pool, deduper, reporter, stats := newTestPool(t, PoolConfig{Workers: 1, QueueSize: 2}, scanner)
	pool.Start()

	jobs := newPoolTestJobs(t)
	first, second := jobs.job("a"), jobs.job("b")
	require.True(t, pool.Submit(first))
	require.True(t, pool.Submit(second))
	jobs.waitAllDone()
	pool.Stop()

	assert.Equal(t, 1, deduper.releasedCount(first.Sum))
	assert.Equal(t, 0, deduper.releasedCount(second.Sum))
	assert.EqualValues(t, 1, stats.ScanErrors.Load())
	assert.EqualValues(t, 1, stats.Scans.Load())
	reports := reportsOf(reporter)
	require.Len(t, reports, 2)
	assert.Error(t, reports[0].err)
	assert.NoError(t, reports[1].err)
}

type scanFunc func(ctx context.Context, data []byte) ([]Match, error)

func (f scanFunc) Scan(ctx context.Context, data []byte) ([]Match, error) { return f(ctx, data) }
func (f scanFunc) RulesVersion() string                                   { return "func" }

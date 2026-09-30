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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compile-time checks that the stand-ins satisfy the contracts
var (
	_ Deduper  = (*StandInDeduper)(nil)
	_ Scanner  = (*MarkerScanner)(nil)
	_ Reporter = (*LogReporter)(nil)
	_ ScanPool = (*InlineScanPool)(nil)
)

type recordingReporter struct {
	mu      sync.Mutex
	reports []report
}

type report struct {
	file    ExecFile
	sum     [32]byte
	matches []Match
	err     error
}

func (r *recordingReporter) Report(f ExecFile, sum [32]byte, matches []Match, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reports = append(r.reports, report{file: f, sum: sum, matches: matches, err: err})
}

type failingScanner struct{}

func (failingScanner) Scan(context.Context, []byte) ([]Match, error) {
	return nil, errors.New("boom")
}

func (failingScanner) RulesVersion() string { return "failing" }

func TestExecFileIdentity(t *testing.T) {
	f := ExecFile{MountID: 1, Inode: 2, CTime: 3, Path: "/bin/true"}
	assert.Equal(t, Identity{MountID: 1, Inode: 2, CTime: 3}, f.Identity())
}

func TestStandInDeduper(t *testing.T) {
	d := NewStandInDeduper()
	now := time.Now()
	id := Identity{MountID: 1, Inode: 2, CTime: 3}

	assert.False(t, d.IdentityFresh(id, now))
	d.MarkIdentity(id, now)
	assert.True(t, d.IdentityFresh(id, now))
	assert.False(t, d.IdentityFresh(Identity{MountID: 1, Inode: 2, CTime: 4}, now), "ctime change must miss")

	sum := sha256.Sum256([]byte("content"))
	assert.True(t, d.ClaimHash(sum))
	assert.False(t, d.ClaimHash(sum))
	d.ReleaseHash(sum)
	assert.True(t, d.ClaimHash(sum))
}

func TestMarkerScanner(t *testing.T) {
	s := NewMarkerScanner("EVIL_MARKER", "test_rule")

	matches, err := s.Scan(context.Background(), []byte("xxEVIL_MARKERxx"))
	require.NoError(t, err)
	assert.Equal(t, []Match{{Rule: "test_rule", Namespace: "standin"}}, matches)

	matches, err = s.Scan(context.Background(), []byte("benign"))
	require.NoError(t, err)
	assert.Empty(t, matches)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.Scan(ctx, []byte("EVIL_MARKER"))
	assert.ErrorIs(t, err, context.Canceled)

	assert.Equal(t, s.RulesVersion(), NewMarkerScanner("EVIL_MARKER", "test_rule").RulesVersion())
	assert.NotEqual(t, s.RulesVersion(), NewMarkerScanner("OTHER", "test_rule").RulesVersion())
}

func TestInlineScanPool(t *testing.T) {
	data := []byte("xxEVIL_MARKERxx")
	sum := sha256.Sum256(data)

	t.Run("match", func(t *testing.T) {
		d := NewStandInDeduper()
		r := &recordingReporter{}
		p := &InlineScanPool{Deduper: d, Scanner: NewMarkerScanner("EVIL_MARKER", "test_rule"), Reporter: r}

		require.True(t, d.ClaimHash(sum))
		done := 0
		assert.True(t, p.Submit(ScanJob{File: ExecFile{Path: "/tmp/evil"}, Sum: sum, Data: data, Done: func() { done++ }}))

		assert.Equal(t, 1, done)
		require.Len(t, r.reports, 1)
		assert.Equal(t, "/tmp/evil", r.reports[0].file.Path)
		assert.Equal(t, sum, r.reports[0].sum)
		assert.Len(t, r.reports[0].matches, 1)
		assert.False(t, d.ClaimHash(sum), "a successful scan keeps the hash claimed")
	})

	t.Run("scan error releases the hash", func(t *testing.T) {
		d := NewStandInDeduper()
		r := &recordingReporter{}
		p := &InlineScanPool{Deduper: d, Scanner: failingScanner{}, Reporter: r}

		require.True(t, d.ClaimHash(sum))
		assert.True(t, p.Submit(ScanJob{Sum: sum, Data: data}))

		require.Len(t, r.reports, 1)
		assert.Error(t, r.reports[0].err)
		assert.True(t, d.ClaimHash(sum), "a failed scan must release the hash so a later exec retries")
	})
}

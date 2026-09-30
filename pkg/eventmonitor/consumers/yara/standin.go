// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// The stand-ins below implement the pipeline contracts with no eviction, no TTL and no native
// code. They exist so that each stage can be built and tested before the others land, and for
// the cgo-free dry run. They are not meant for production use.

// StandInDeduper is an unbounded, in-memory Deduper. Identities never expire.
type StandInDeduper struct {
	mu         sync.Mutex
	identities map[Identity]time.Time
	hashes     map[[32]byte]struct{}
}

// NewStandInDeduper returns a new StandInDeduper
func NewStandInDeduper() *StandInDeduper {
	return &StandInDeduper{
		identities: make(map[Identity]time.Time),
		hashes:     make(map[[32]byte]struct{}),
	}
}

// IdentityFresh implements Deduper
func (d *StandInDeduper) IdentityFresh(id Identity, _ time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.identities[id]
	return ok
}

// MarkIdentity implements Deduper
func (d *StandInDeduper) MarkIdentity(id Identity, now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.identities[id] = now
}

// ClaimHash implements Deduper
func (d *StandInDeduper) ClaimHash(sum [32]byte) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.hashes[sum]; ok {
		return false
	}
	d.hashes[sum] = struct{}{}
	return true
}

// ReleaseHash implements Deduper
func (d *StandInDeduper) ReleaseHash(sum [32]byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.hashes, sum)
}

// MarkerScanner is a Scanner that reports a match when the data contains Marker
type MarkerScanner struct {
	Marker []byte
	Rule   string
}

// NewMarkerScanner returns a MarkerScanner reporting rule when data contains marker
func NewMarkerScanner(marker string, rule string) *MarkerScanner {
	return &MarkerScanner{
		Marker: []byte(marker),
		Rule:   rule,
	}
}

// Scan implements Scanner
func (s *MarkerScanner) Scan(ctx context.Context, data []byte) ([]Match, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(s.Marker) == 0 || !bytes.Contains(data, s.Marker) {
		return nil, nil
	}
	return []Match{{Rule: s.Rule, Namespace: "standin"}}, nil
}

// RulesVersion implements Scanner
func (s *MarkerScanner) RulesVersion() string {
	sum := sha256.Sum256(append([]byte(s.Rule+"\x00"), s.Marker...))
	return "standin-" + hex.EncodeToString(sum[:8])
}

// LogReporter is a Reporter that writes one log line per report
type LogReporter struct {
	RulesVersion string
}

// Report implements Reporter
func (r *LogReporter) Report(f ExecFile, sum [32]byte, matches []Match, err error) {
	if err != nil {
		log.Warnf("yara: scan failed path=%s pid=%d container_id=%s sha256=%x rules_version=%s: %v",
			f.Path, f.PID, f.ContainerID, sum, r.RulesVersion, err)
		return
	}
	if len(matches) == 0 {
		log.Debugf("yara: no match path=%s pid=%d container_id=%s sha256=%x rules_version=%s",
			f.Path, f.PID, f.ContainerID, sum, r.RulesVersion)
		return
	}
	for _, m := range matches {
		log.Infof("yara: match path=%s pid=%d container_id=%s sha256=%x rule=%s namespace=%s tags=%v rules_version=%s",
			f.Path, f.PID, f.ContainerID, sum, m.Rule, m.Namespace, m.Tags, r.RulesVersion)
	}
}

// InlineScanPool is a ScanPool that scans synchronously in Submit. Unlike a real pool, Submit
// blocks for the whole scan, so it is only suitable for tests and the dry run.
type InlineScanPool struct {
	Deduper  Deduper
	Scanner  Scanner
	Reporter Reporter
	Timeout  time.Duration
}

// Submit implements ScanPool. It always accepts the job.
func (p *InlineScanPool) Submit(job ScanJob) bool {
	if job.Done != nil {
		defer job.Done()
	}

	ctx := context.Background()
	if p.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.Timeout)
		defer cancel()
	}

	matches, err := p.Scanner.Scan(ctx, job.Data)
	if err != nil {
		p.Deduper.ReleaseHash(job.Sum)
	}
	p.Reporter.Report(job.File, job.Sum, matches, err)
	return true
}

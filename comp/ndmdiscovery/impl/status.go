// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"embed"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Range states shown on the status page.
const (
	stateScanning = "scanning"
	statePaused   = "paused"
	stateWaiting  = "waiting"
	stateFailed   = "failed"
	statePending  = "pending"
)

// statusProvider renders the NDM Discovery section of the agent status.
type statusProvider struct {
	d   *ndmDiscovery
	now func() time.Time
}

// discoveryStatus is the NDM Discovery section of the agent status.
type discoveryStatus struct {
	Workers  int64
	Ranges   []rangeStatus
	Rejected []rejectedRange
}

// rangeStatus is one scheduled range on the status page.
type rangeStatus struct {
	ID             string
	NetworkAddress string
	Namespace      string
	IntervalSec    int
	Probes         string
	State          string
	RunID          string
	Scanned        int64
	Total          int64
	ProgressPct    int64
	StartedAt      string
	EndedAt        string
	NextRunAt      string
	NextRunIn      string
	Issues         []string
}

// Name returns the name of the status section.
func (statusProvider) Name() string { return "NDM Discovery" }

// Section returns the section the provider is grouped under.
func (statusProvider) Section() string { return "NDM Discovery" }

// JSON adds the section to the status map.
func (p statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	stats["ndmDiscoveryStats"] = p.status()
	return nil
}

// Text renders the section as text.
func (p statusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "ndmdiscovery.tmpl", buffer, p.data())
}

// HTML renders the section as HTML.
func (p statusProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "ndmdiscoveryHTML.tmpl", buffer, p.data())
}

func (p statusProvider) data() map[string]interface{} {
	return map[string]interface{}{"ndmDiscoveryStats": p.status()}
}

func (p statusProvider) status() discoveryStatus {
	now := time.Now
	if p.now != nil {
		now = p.now
	}

	p.d.mu.Lock()
	rejected := append([]rejectedRange(nil), p.d.rejected...)
	p.d.mu.Unlock()
	sort.Slice(rejected, func(i, j int) bool { return rejected[i].ID < rejected[j].ID })

	snapshots := p.d.sched.snapshot()
	ranges := make([]rangeStatus, 0, len(snapshots))
	for _, snap := range snapshots {
		ranges = append(ranges, p.rangeStatus(snap, now()))
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].NetworkAddress != ranges[j].NetworkAddress {
			return ranges[i].NetworkAddress < ranges[j].NetworkAddress
		}
		return ranges[i].ID < ranges[j].ID
	})

	return discoveryStatus{
		Workers:  p.d.sched.opts.Workers,
		Ranges:   ranges,
		Rejected: rejected,
	}
}

func (p statusProvider) rangeStatus(snap rangeSnapshot, now time.Time) rangeStatus {
	cfg := snap.cfg
	rs := rangeStatus{
		ID:             cfg.AutodiscoveryID,
		NetworkAddress: cfg.NetworkAddress,
		Namespace:      cfg.Namespace,
		IntervalSec:    cfg.IntervalSec,
		Probes:         strings.Join(probeKinds(cfg.Probes), ", "),
		State:          statePending,
		Issues:         snap.issues,
	}

	if plan, err := newChunkPlan(cfg.NetworkAddress, cfg.IgnoredIPAddresses, p.d.sched.opts.MaxAddresses); err == nil {
		rs.Total = int64(plan.totalAddresses())
	}

	if p.d.sched.sweeper == nil {
		return rs
	}
	cursor, ok := p.d.sched.sweeper.cursors.Load(cfg.AutodiscoveryID)
	if !ok || cursor.RunID == "" {
		if snap.sweeping {
			rs.State = stateScanning
		}
		return rs
	}

	rs.RunID = cursor.RunID
	rs.Scanned = cursor.Scanned
	if rs.Total > 0 {
		rs.ProgressPct = min(100, cursor.Scanned*100/rs.Total)
	}
	if cursor.StartedAtMs > 0 {
		rs.StartedAt = formatTime(time.UnixMilli(cursor.StartedAtMs))
	}

	if snap.sweeping {
		rs.State = stateScanning
		return rs
	}
	if cursor.EndedAtMs == 0 {
		rs.State = statePaused
		return rs
	}

	rs.State = stateWaiting
	if cursor.Failed {
		rs.State = stateFailed
	}
	rs.EndedAt = formatTime(time.UnixMilli(cursor.EndedAtMs))
	next := time.UnixMilli(cursor.EndedAtMs).Add(time.Duration(cfg.IntervalSec) * time.Second)
	rs.NextRunAt = formatTime(next)
	if wait := next.Sub(now); wait > 0 {
		rs.NextRunIn = wait.Truncate(time.Second).String()
	} else {
		rs.NextRunIn = "due"
	}
	return rs
}

func probeKinds(p probeParams) []string {
	var kinds []string
	if p.Ping != nil {
		kinds = append(kinds, kindPing)
	}
	if p.SNMP != nil {
		kinds = append(kinds, kindSNMP)
	}
	return kinds
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

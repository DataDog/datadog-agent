// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/semaphore"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe"
)

// Reachability statuses reported per address.
const (
	statusReachable   = "reachable"
	statusUnreachable = "unreachable"
)

// scanFunc probes a batch of addresses. It is satisfied by probe.Scan.
type scanFunc func(ctx context.Context, workers int, targets []string, opts probe.Options) ([]probe.Result, error)

// sweepRequest is everything one cycle over one range needs.
type sweepRequest struct {
	Config rangeConfig
	// Options are this cycle's resolved probes.
	Options probe.Options
	Plan    *chunkPlan
	Digest  string
	// Workers is this range's share of the global worker budget.
	Workers int64
}

// sweeper runs one cycle over one range, chunk by chunk, persisting a cursor
// so that a restart resumes instead of starting again.
type sweeper struct {
	scan     scanFunc
	reporter discoveryReporter
	cursors  cursorStore
	sem      *semaphore.Weighted
	// budget is the size of sem.
	budget int64
	log    log.Component

	now      func() int64
	newRunID func() string
}

func newSweeper(scan scanFunc, reporter discoveryReporter, cursors cursorStore, sem *semaphore.Weighted, budget int64, logger log.Component) *sweeper {
	if budget < 1 {
		budget = 1
	}
	return &sweeper{
		scan:     scan,
		reporter: reporter,
		cursors:  cursors,
		sem:      sem,
		budget:   budget,
		log:      logger,
		now:      func() int64 { return time.Now().UnixMilli() },
		newRunID: func() string { return uuid.New().String() },
	}
}

func (s *sweeper) sweep(ctx context.Context, r sweepRequest) error {
	id := r.Config.AutodiscoveryID
	r.Workers = clampWorkers(r.Workers, s.budget)
	state := s.startState(r)
	total := r.Plan.chunkCount()

	if state.NextChunk == 0 {
		s.log.Infof("ndmdiscovery: scanning range %s (%s): %d addresses in %d chunks, %d ignored, run %s",
			id, r.Config.CIDR, r.Plan.totalAddresses(), total, r.Plan.ignoredCount(), state.RunID)
		s.reportRun(r, metadata.AutodiscoveryRunMetadata{
			AutodiscoveryID:  id,
			RunID:            state.RunID,
			Status:           metadata.AutodiscoveryRunInProgress,
			AddressesScanned: state.Scanned,
			StartedAtMs:      state.StartedAtMs,
		})
	} else {
		s.log.Infof("ndmdiscovery: resuming the scan of range %s (%s) at chunk %d of %d, run %s",
			id, r.Config.CIDR, state.NextChunk, total, state.RunID)
	}

	// reported is a lower bound after a restart: a resumed run inherits no count.
	reported := 0
	for state.NextChunk < total {
		chunk := r.Plan.chunk(state.NextChunk)

		devices, err := s.probe(ctx, r, state.RunID, chunk)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				// The run is paused, not broken, so the next start resumes it.
				s.saveCursor(id, state)
				return err
			}
			// The cursor is kept to resume at this chunk, marked failed so
			// that the resume opens a new run.
			state.Failed = true
			s.saveCursor(id, state)
			s.reportRun(r, metadata.AutodiscoveryRunMetadata{
				AutodiscoveryID:  id,
				RunID:            state.RunID,
				Status:           metadata.AutodiscoveryRunFailed,
				AddressesScanned: state.Scanned,
				Error:            err.Error(),
				StartedAtMs:      state.StartedAtMs,
				FinishedAtMs:     s.now(),
			})
			return err
		}

		if len(devices) > 0 {
			if err := s.reporter.ReportDevices(r.Config.Namespace, devices); err != nil {
				// A transport failure is not a scan failure.
				s.log.Warnf("ndmdiscovery: failed to report chunk %d of range %s: %v", chunk.Index, id, err)
			} else {
				reported += len(devices)
			}
		}

		state.NextChunk++
		state.Scanned += int64(len(chunk.Targets))
		s.saveCursor(id, state)
	}

	s.log.Infof("ndmdiscovery: completed the scan of range %s (%s): %d addresses scanned, %d devices reported, run %s",
		id, r.Config.CIDR, state.Scanned, reported, state.RunID)
	s.reportRun(r, metadata.AutodiscoveryRunMetadata{
		AutodiscoveryID:  id,
		RunID:            state.RunID,
		Status:           metadata.AutodiscoveryRunCompleted,
		AddressesScanned: state.Scanned,
		StartedAtMs:      state.StartedAtMs,
		FinishedAtMs:     s.now(),
	})

	if err := s.cursors.Clear(id); err != nil {
		s.log.Warnf("ndmdiscovery: failed to clear the cursor of range %s: %v", id, err)
	}
	return nil
}

// startState resumes the persisted cycle when the range and its credentials
// are unchanged, and starts a new one otherwise.
func (s *sweeper) startState(r sweepRequest) cursorState {
	if saved, ok := s.cursors.Load(r.Config.AutodiscoveryID); ok &&
		saved.ConfigDigest == r.Digest &&
		saved.NextChunk > 0 &&
		saved.NextChunk < r.Plan.chunkCount() {
		if !saved.Failed {
			return saved
		}

		// The saved run already reported a terminal status, so the remaining
		// chunks continue under a fresh run that carries over its progress.
		saved.RunID = s.newRunID()
		saved.StartedAtMs = s.now()
		saved.Failed = false
		s.saveCursor(r.Config.AutodiscoveryID, saved)
		s.reportRun(r, metadata.AutodiscoveryRunMetadata{
			AutodiscoveryID:  r.Config.AutodiscoveryID,
			RunID:            saved.RunID,
			Status:           metadata.AutodiscoveryRunInProgress,
			AddressesScanned: saved.Scanned,
			StartedAtMs:      saved.StartedAtMs,
		})
		return saved
	}

	return cursorState{
		RunID:     s.newRunID(),
		NextChunk: 0,
		// Ignored addresses are counted up front so the total is still reached.
		Scanned:      int64(r.Plan.ignoredCount()),
		StartedAtMs:  s.now(),
		ConfigDigest: r.Digest,
	}
}

func (s *sweeper) probe(ctx context.Context, r sweepRequest, runID string, chunk probeChunk) ([]metadata.DiscoveredDeviceMetadata, error) {
	if len(chunk.Targets) == 0 {
		// Every address of this chunk is ignored, so there is nothing to probe.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	// The worker budget is global, so a chunk waits for its share.
	if err := s.sem.Acquire(ctx, r.Workers); err != nil {
		return nil, err
	}
	defer s.sem.Release(r.Workers)

	res, err := s.scan(ctx, int(r.Workers), chunk.Targets, r.Options)
	if err != nil {
		return nil, err
	}
	return toDiscoveredDevices(r.Config.AutodiscoveryID, runID, res), nil
}

// clampWorkers keeps a range's worker share inside [1, budget]: a larger share
// can never be acquired, and a zero share bounds nothing.
func clampWorkers(workers, budget int64) int64 {
	if workers < 1 {
		return 1
	}
	if workers > budget {
		return budget
	}
	return workers
}

func (s *sweeper) saveCursor(id string, state cursorState) {
	if err := s.cursors.Save(id, state); err != nil {
		s.log.Warnf("ndmdiscovery: failed to persist the cursor of range %s: %v", id, err)
	}
}

func (s *sweeper) reportRun(r sweepRequest, run metadata.AutodiscoveryRunMetadata) {
	if err := s.reporter.ReportRun(r.Config.Namespace, run); err != nil {
		s.log.Warnf("ndmdiscovery: failed to report the run status of range %s: %v", r.Config.AutodiscoveryID, err)
	}
}

// toDiscoveredDevices converts one chunk's probe results into report documents.
// Only addresses that answered at least one probe are reported.
func toDiscoveredDevices(autodiscoveryID, runID string, results []probe.Result) []metadata.DiscoveredDeviceMetadata {
	devices := make([]metadata.DiscoveredDeviceMetadata, 0, len(results))
	for _, r := range results {
		if r.Target == "" {
			continue
		}

		device := metadata.DiscoveredDeviceMetadata{
			AutodiscoveryID: autodiscoveryID,
			RunID:           runID,
			IPAddress:       r.Target,
		}
		answered := false

		if p := r.Ping; p != nil {
			result := metadata.ProbeResult{
				Kind:   kindPing,
				Status: statusString(p.Success),
				RttMs:  rttMs(p.Success, p.RTT),
			}
			if p.Success {
				answered = true
			} else {
				result.FailureReason = p.FailureReason
			}
			device.ProbeResults = append(device.ProbeResults, result)
		}

		if sn := r.SNMP; sn != nil {
			result := metadata.ProbeResult{
				Kind:   kindSNMP,
				Status: statusString(sn.Success),
				RttMs:  rttMs(sn.Success, sn.RTT),
			}
			if sn.Success {
				answered = true
				result.CredID = sn.CredID
				if device.Name == "" {
					device.Name = sn.SysName
				}
			} else {
				result.FailureReason = sn.FailureReason
			}
			device.ProbeResults = append(device.ProbeResults, result)
		}

		if !answered {
			continue
		}
		devices = append(devices, device)
	}
	return devices
}

func statusString(success bool) string {
	if success {
		return statusReachable
	}
	return statusUnreachable
}

// rttMs is the round-trip time in milliseconds, reported only when the probe
// answered.
func rttMs(success bool, rtt time.Duration) *int64 {
	if !success {
		return nil
	}
	ms := rtt.Milliseconds()
	return &ms
}

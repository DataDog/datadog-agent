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
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// Reachability statuses reported per address.
const (
	statusReachable   = "reachable"
	statusUnreachable = "unreachable"
)

// sweepRequest is everything one cycle over one range needs.
type sweepRequest struct {
	Config      rangeConfig
	Credentials []connectivity.SNMPCredential
	Plan        *chunkPlan
	Digest      string
	// Workers is this range's share of the global worker budget.
	Workers int64
	// PingEnabled is false when the agent cannot send ICMP, so ping_status is
	// left empty rather than reported as unreachable for every address.
	PingEnabled bool
}

// sweeper runs one cycle over one range: chunk by chunk, reporting as it goes
// and persisting a cursor so a restart resumes instead of starting again.
type sweeper struct {
	checker  connectivityChecker
	reporter discoveryReporter
	cursors  cursorStore
	sem      *semaphore.Weighted
	// budget is the size of sem. Acquiring more than that blocks until the
	// context is done, so a range's worker share is clamped to it.
	budget int64
	log    log.Component

	now      func() int64
	newRunID func() string
}

func newSweeper(checker connectivityChecker, reporter discoveryReporter, cursors cursorStore, sem *semaphore.Weighted, budget int64, logger log.Component) *sweeper {
	if budget < 1 {
		budget = 1
	}
	return &sweeper{
		checker:  checker,
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
	// A share below 1 bounds nothing, and a share above the global budget can
	// never be acquired, so it is clamped rather than left to hang the sweep.
	r.Workers = clampWorkers(r.Workers, s.budget)
	state := s.startState(r)
	total := r.Plan.chunkCount()

	// One line per cycle, not per chunk: a /16 is 256 chunks. This and the
	// completion line below are the only positive-path signals the component
	// emits, so between them they have to say what is being scanned, how far it
	// got, and which run the backend should be showing.
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

	// reported counts the whole cycle, including the chunks a resumed run
	// inherited nothing from, so it is a lower bound after a restart.
	reported := 0
	for state.NextChunk < total {
		chunk := r.Plan.chunk(state.NextChunk)

		devices, err := s.probe(ctx, r, state.RunID, chunk)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				// The agent is stopping. The run is paused, not broken, so the
				// cursor is kept as is and the next start resumes this run.
				s.saveCursor(id, state)
				return err
			}
			// This run ends here on a terminal failed record. The cursor is
			// kept so the next tick resumes at this same chunk instead of
			// re-scanning the range, but it is marked failed so that the
			// resume opens a new run rather than completing one the backend
			// already recorded as failed.
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
				// A transport failure is not a scan failure: log it and keep
				// sweeping rather than aborting a multi-hour cycle.
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

		// The saved run already ended on a terminal failed record. Finishing it
		// would give the backend a second terminal record for one run ID, so
		// the remaining chunks continue under a fresh run while the progress
		// made so far is preserved. That carried-over progress is what makes
		// AddressesScanned cycle-cumulative rather than per-run: the new run
		// reports the whole cycle's count, so summing the runs of a cycle
		// double-counts. See AutodiscoveryRunMetadata.AddressesScanned.
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
		// Ignored addresses are counted up front so that a range with
		// exclusions still reaches its total.
		Scanned:      int64(r.Plan.ignoredCount()),
		StartedAtMs:  s.now(),
		ConfigDigest: r.Digest,
	}
}

func (s *sweeper) probe(ctx context.Context, r sweepRequest, runID string, chunk probeChunk) ([]metadata.DiscoveredDeviceMetadata, error) {
	if len(chunk.Targets) == 0 {
		// Every address of this chunk is ignored, so there is nothing to
		// probe. A cancelled context still stops the cycle here.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	// The worker budget is global, so a chunk waits for its share rather than
	// each range running its own pool.
	if err := s.sem.Acquire(ctx, r.Workers); err != nil {
		return nil, err
	}
	defer s.sem.Release(r.Workers)

	req := connectivity.Request{
		Targets:     chunk.Targets,
		Checks:      []string{connectivity.CheckSNMP},
		SNMPOptions: r.Config.SNMPOptions,
		Credentials: r.Credentials,
		Workers:     int(r.Workers),
	}
	if r.PingEnabled && r.Config.PingOptions != nil {
		// Ping first: it is the cheaper probe and its result is reported
		// alongside the SNMP result for the same address.
		req.Checks = []string{connectivity.CheckPing, connectivity.CheckSNMP}
		req.PingOptions = r.Config.PingOptions
	}

	res, err := s.checker.CheckConnectivity(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return toDiscoveredDevices(r.Config.AutodiscoveryID, runID, res), nil
}

// clampWorkers keeps a range's worker share inside [1, budget].
// semaphore.Weighted.Acquire blocks until the context is done when n exceeds
// the semaphore size, and bounds nothing at all when n is zero.
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

// toDiscoveredDevices converts one chunk's probe results into report
// documents. Only addresses that answered a check are reported: a silent
// address is absent from the run rather than reported as unreachable, and the
// backend reads that absence against the run ID. Reporting every probed
// address instead would put 65536 documents on the metadata stream per /16
// cycle to say that almost all of them are empty.
//
// One answered check is enough. An address that answers ping but refuses SNMP
// is a device the range's credentials do not open, which is exactly what the
// approval UI has to tell apart from an empty address.
func toDiscoveredDevices(autodiscoveryID, runID string, res connectivity.Result) []metadata.DiscoveredDeviceMetadata {
	devices := make([]metadata.DiscoveredDeviceMetadata, 0, len(res.Devices))
	for _, d := range res.Devices {
		if d.IPAddress == "" {
			// The engine pre-allocates its result slice, so an interrupted run
			// can leave zero-value holes.
			continue
		}
		pingAnswered := d.PingResult != nil && d.PingResult.Success
		snmpAnswered := d.SNMPResult != nil && d.SNMPResult.Success
		if !pingAnswered && !snmpAnswered {
			continue
		}

		device := metadata.DiscoveredDeviceMetadata{
			AutodiscoveryID: autodiscoveryID,
			RunID:           runID,
			IPAddress:       d.IPAddress,
		}
		if d.PingResult != nil {
			device.PingStatus = statusString(d.PingResult.Success)
		}
		if d.SNMPResult != nil {
			device.SNMPStatus = statusString(d.SNMPResult.Success)
			if d.SNMPResult.Success {
				device.Name = d.SNMPResult.SysName
				device.SNMPCredID = d.SNMPResult.CredID
			}
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helmactionsimpl

import (
	"context"
	"sync"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"

	helmactions "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// ActionTTL is how long action timestamps are considered valid.
	ActionTTL = 1 * time.Minute
	// RecordRetentionTTL is how long action records are kept in memory.
	RecordRetentionTTL = 24 * time.Hour
	// CleanupInterval is how often expired records are purged.
	CleanupInterval = 30 * time.Second
)

// JobPhase summarises a tracked Job's high-level state.
type JobPhase string

const (
	// JobPhasePending — Job created, no completion condition yet.
	JobPhasePending JobPhase = "pending"
	// JobPhaseRunning — at least one pod active.
	JobPhaseRunning JobPhase = "running"
	// JobPhaseSucceeded — Job has a Complete condition.
	JobPhaseSucceeded JobPhase = "succeeded"
	// JobPhaseFailed — Job has a Failed condition or exceeded backoffLimit.
	JobPhaseFailed JobPhase = "failed"
)

// JobRecord captures the latest observed state of a tracked rollback Job.
type JobRecord struct {
	UID         types.UID
	Namespace   string
	Name        string
	Phase       JobPhase
	Active      int32
	Succeeded   int32
	Failed      int32
	Message     string
	CreatedAt   int64     // unix seconds, time we started tracking
	UpdatedAt   int64     // unix seconds, last watch event time
	CompletedAt int64     // unix seconds, 0 until succeeded/failed
	ReportedTs  time.Time // When final status report was sent to EVP

	// Action metadata copied from RollbackInputs at TrackJob time and carried
	// forward, unchanged, through every UpdateJob rebuild (same treatment as
	// CreatedAt) — needed to report completion back to EVP against the
	// originating task once the Job reaches a terminal state.
	ActionID         string
	OrgID            int64
	Release          string
	ReleaseNamespace string
}

func (r *JobRecord) phaseIsTerminal() bool {
	return r.Phase == JobPhaseFailed || r.Phase == JobPhaseSucceeded
}

func (r *JobRecord) reported() bool {
	return !r.ReportedTs.IsZero()
}

func (r *JobRecord) markReported() {
	// update only if not set before.
	if r.ReportedTs.IsZero() {
		r.ReportedTs = time.Now()
	}
}

// ActionStore tracks processed actions in-memory to prevent duplicate execution.
type ActionStore struct {
	// mu guards jobs
	mu       sync.RWMutex
	jobs     map[types.UID]*JobRecord
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewActionStore creates a new ActionStore and starts the background cleanup goroutine.
func NewActionStore() *ActionStore {
	s := &ActionStore{
		jobs: make(map[types.UID]*JobRecord),
	}

	log.Debugf("[HelmActions] Action store initialized (retention=%v, cleanup=%v)", RecordRetentionTTL, CleanupInterval)
	return s
}

// trackedLifecycle is the union of record types tracked by (namespace, name,
// UID) with a created/updated/completed timestamp lifecycle. Kept as a type
// union rather than an interface because JobRecord and PodRecord otherwise
// share no methods — the shared shape is purely structural.
type trackedLifecycle interface {
	JobRecord
}

// upsertTracked centralises the lock/lookup/write shell used by both
// UpdateJob and UpdatePod. The build callback receives the previous record
// (zero value if none) and the current unix time, and must:
//   - preserve prev.CreatedAt, prev.CompletedAt, and any other carry-over
//     fields (e.g. PodRecord.Logs);
//   - stamp UpdatedAt = now;
//   - set CreatedAt = now if the record is new;
//   - stamp CompletedAt = now on entry into a terminal phase.
//
// The bool returned by build() is passed through unchanged — semantics differ
// per record type (Job: "just terminal", Pod: "just failed"), and only the
// caller knows which transition matters to its watcher.
func upsertTracked[T trackedLifecycle](
	s *ActionStore,
	m map[types.UID]*T,
	uid types.UID,
	build func(prev *T, now int64) *T,
) *T {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := m[uid]
	if prev == nil {
		prev = new(T)
	}
	now := time.Now().Unix()
	rec := build(prev, now)
	m[uid] = rec
	return rec
}

// TrackJob registers a Job for status tracking. Idempotent: a second call with
// the same UID is a no-op (the watcher will own subsequent updates).
func (s *ActionStore) TrackJob(job *batchv1.Job, in *helmactions.RollbackInputs, meta helmactions.TaskMeta) {
	if job == nil || job.UID == "" {
		return
	}

	now := time.Now().Unix()
	rec := &JobRecord{
		UID:              job.UID,
		Namespace:        job.Namespace,
		Name:             job.Name,
		Phase:            JobPhasePending,
		CreatedAt:        now,
		UpdatedAt:        now,
		ActionID:         meta.ActionID,
		OrgID:            meta.OrgID,
		Release:          in.Release,
		ReleaseNamespace: in.ReleaseNamespace,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if old, exists := s.jobs[job.UID]; exists {
		log.Debugf("[HelmActions] Tracking Job that already exists: %s", job.UID)
		// If job is already there it means tracking loop added it earlier than this call.
		// Update all fields except time related
		rec.CreatedAt = old.CreatedAt
		rec.UpdatedAt = old.UpdatedAt
	}

	s.jobs[job.UID] = rec
	log.Debugf("[HelmActions] Tracking Job %s/%s (uid=%s, actionID=%s)", job.Namespace, job.Name, job.UID, meta.ActionID)
}

// UpdateJob applies the latest observed state of a Job to the store. Called by
// the Job watcher on ADDED/MODIFIED events. Returns the resulting record and
// whether it represents a transition into a terminal phase (succeeded/failed).
func (s *ActionStore) UpdateJob(job *batchv1.Job) *JobRecord {
	return upsertTracked(s, s.jobs, job.UID, func(prev *JobRecord, now int64) *JobRecord {
		actionID := jobActionID(job, prev.ActionID)

		phase, msg := classifyJob(job)
		rec := &JobRecord{
			UID:              job.UID,
			Namespace:        job.Namespace,
			Name:             job.Name,
			Phase:            phase,
			Active:           job.Status.Active,
			Succeeded:        job.Status.Succeeded,
			Failed:           job.Status.Failed,
			Message:          msg,
			CreatedAt:        prev.CreatedAt,
			UpdatedAt:        now,
			CompletedAt:      prev.CompletedAt,
			ReportedTs:       prev.ReportedTs,
			ActionID:         actionID,
			OrgID:            prev.OrgID,
			Release:          prev.Release,
			ReleaseNamespace: prev.ReleaseNamespace,
		}
		if prev.CreatedAt == 0 {
			// Watcher saw the Job before OnRollback ran (relisted on reconnect).
			rec.CreatedAt = now
		}
		if rec.CompletedAt == 0 && (phase == JobPhaseSucceeded || phase == JobPhaseFailed) {
			rec.CompletedAt = now
		}
		return rec
	})
}

// jobActionID determines actionID for current job usign following logic:
// in normal conditions DCA runs for a long time and prevID is always present when job is not manually created.
// prevID can be "" on DCA restart in which case functions inspects job annotation for actionID.
func jobActionID(job *batchv1.Job, prevID string) string {
	if prevID != "" {
		return prevID
	}
	if job.Annotations == nil {
		return ""
	}
	return job.Annotations[helmactions.AnnotationActionID]
}

// RemoveJob drops a tracked Job. Called on watcher DELETED events.
func (s *ActionStore) RemoveJob(uid types.UID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, uid)
}

// classifyJob derives a high-level phase + summary message from a Job's Status
// conditions. Helm's Job is expected to either Complete or fail (Failed
// condition or backoffLimit hit).
func classifyJob(job *batchv1.Job) (JobPhase, string) {
	for _, c := range job.Status.Conditions {
		if c.Status != "True" {
			continue
		}
		switch c.Type {
		case batchv1.JobComplete, batchv1.JobSuccessCriteriaMet:
			return JobPhaseSucceeded, c.Message
		case batchv1.JobFailed:
			return JobPhaseFailed, c.Message
		}
	}
	if job.Status.Active > 0 {
		return JobPhaseRunning, ""
	}
	return JobPhasePending, ""
}

func (s *ActionStore) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Debugf("[HelmActions] Action store cleanup loop stopped")
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

func (s *ActionStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-RecordRetentionTTL).Unix()

	removedJobs := 0
	for uid, j := range s.jobs {
		// Drop terminal Jobs that have been finished longer than the retention
		// window. Active Jobs are kept regardless of age — they are the point
		// of the tracking.
		if j.CompletedAt > 0 && j.CompletedAt < cutoff {
			delete(s.jobs, uid)
			removedJobs++
		}
	}
	if removedJobs > 0 {
		log.Debugf("[HelmActions] Cleaned up %d completed Job records (remaining: %d)", removedJobs, len(s.jobs))
	}
}

func (s *ActionStore) RunCleanup(ctx context.Context) {
	s.stopCh = make(chan struct{})
	go s.cleanupLoop(ctx)
}

// Stop shuts down the cleanup goroutine.
func (s *ActionStore) StopCleanup() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

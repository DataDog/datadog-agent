// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helmactionsimpl

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"

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

// ActionRecord stores information about a processed action.
type ActionRecord struct {
	Key             ActionKey
	Status          string
	Message         string
	ExecutedAt      int64
	ReceivedAt      int64
	ActionCreatedAt int64
	ClaimedAt       int64
}

// ActionStoreInterface defines the store methods used by ActionProcessor.
type ActionStoreInterface interface {
	Claim(key ActionKey) bool
	MarkExecuted(key ActionKey, status, message string, executedAt, receivedAt, actionCreatedAt int64)
	GetRecord(key ActionKey) (ActionRecord, bool)
}

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
	CreatedAt   int64 // unix seconds, time we started tracking
	UpdatedAt   int64 // unix seconds, last watch event time
	CompletedAt int64 // unix seconds, 0 until succeeded/failed
}

// ActionStore tracks processed actions in-memory to prevent duplicate execution.
type ActionStore struct {
	executed map[string]ActionRecord
	jobs     map[types.UID]JobRecord
	mu       sync.RWMutex
	stopCh   chan struct{}
}

var _ ActionStoreInterface = (*ActionStore)(nil)

// NewActionStore creates a new ActionStore and starts the background cleanup goroutine.
func NewActionStore(ctx context.Context) *ActionStore {
	s := &ActionStore{
		executed: make(map[string]ActionRecord),
		jobs:     make(map[types.UID]JobRecord),
		stopCh:   make(chan struct{}),
	}
	go s.cleanupLoop(ctx)
	log.Debugf("[HelmActions] Action store initialized (TTL=%v, retention=%v, cleanup=%v)",
		ActionTTL, RecordRetentionTTL, CleanupInterval)
	return s
}

// ValidateTimestamp returns an error if ts is zero, in the future, or older than ActionTTL.
func ValidateTimestamp(ts time.Time) error {
	if ts.IsZero() {
		return errors.New("action timestamp is missing or zero")
	}
	now := time.Now()
	if ts.After(now.Add(10 * time.Second)) {
		return fmt.Errorf("action timestamp is in the future: %v (now: %v)", ts, now)
	}
	if time.Since(ts) > ActionTTL {
		return fmt.Errorf("action timestamp is expired: %v (age: %v, TTL: %v)", ts, time.Since(ts), ActionTTL)
	}
	return nil
}

// Claim tries to claim an action for execution. Returns false if already claimed.
func (s *ActionStore) Claim(key ActionKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.executed[key.String()]; exists {
		return false
	}
	s.executed[key.String()] = ActionRecord{
		Key:       key,
		Status:    StatusClaimed,
		Message:   "action claimed",
		ClaimedAt: time.Now().Unix(),
	}
	return true
}

// MarkExecuted updates the record for a previously claimed action.
func (s *ActionStore) MarkExecuted(key ActionKey, status, message string, executedAt, receivedAt, actionCreatedAt int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	claimedAt := s.executed[key.String()].ClaimedAt
	s.executed[key.String()] = ActionRecord{
		Key:             key,
		Status:          status,
		Message:         message,
		ExecutedAt:      executedAt,
		ReceivedAt:      receivedAt,
		ActionCreatedAt: actionCreatedAt,
		ClaimedAt:       claimedAt,
	}
}

// GetRecord retrieves the execution record for an action.
func (s *ActionStore) GetRecord(key ActionKey) (ActionRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, exists := s.executed[key.String()]
	return record, exists
}

// GetAll returns all execution records.
func (s *ActionStore) GetAll() []ActionRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Collect(maps.Values(s.executed))
}

// Count returns the number of tracked action records.
func (s *ActionStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.executed)
}

// TrackJob registers a Job for status tracking. Idempotent: a second call with
// the same UID is a no-op (the watcher will own subsequent updates).
func (s *ActionStore) TrackJob(job *batchv1.Job) {
	if job == nil || job.UID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.UID]; exists {
		return
	}
	now := time.Now().Unix()
	s.jobs[job.UID] = JobRecord{
		UID:       job.UID,
		Namespace: job.Namespace,
		Name:      job.Name,
		Phase:     JobPhasePending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	log.Debugf("[HelmActions] Tracking Job %s/%s (uid=%s)", job.Namespace, job.Name, job.UID)
}

// UpdateJob applies the latest observed state of a Job to the store. Called by
// the Job watcher on ADDED/MODIFIED events. Returns the resulting record and
// whether it represents a transition into a terminal phase (succeeded/failed).
func (s *ActionStore) UpdateJob(job *batchv1.Job) (JobRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, existed := s.jobs[job.UID]
	now := time.Now().Unix()
	phase, msg := classifyJob(job)

	rec := JobRecord{
		UID:         job.UID,
		Namespace:   job.Namespace,
		Name:        job.Name,
		Phase:       phase,
		Active:      job.Status.Active,
		Succeeded:   job.Status.Succeeded,
		Failed:      job.Status.Failed,
		Message:     msg,
		CreatedAt:   prev.CreatedAt,
		UpdatedAt:   now,
		CompletedAt: prev.CompletedAt,
	}
	if !existed {
		// Watcher saw the Job before OnRollback ran (relisted on reconnect).
		rec.CreatedAt = now
	}
	if rec.CompletedAt == 0 && (phase == JobPhaseSucceeded || phase == JobPhaseFailed) {
		rec.CompletedAt = now
	}
	s.jobs[job.UID] = rec

	terminal := rec.CompletedAt > 0 && prev.CompletedAt == 0
	return rec, terminal
}

// RemoveJob drops a tracked Job. Called on watcher DELETED events.
func (s *ActionStore) RemoveJob(uid types.UID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, uid)
}

// GetJob returns a tracked Job record by UID.
func (s *ActionStore) GetJob(uid types.UID) (JobRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.jobs[uid]
	return rec, ok
}

// GetAllJobs returns a snapshot of all tracked Job records.
func (s *ActionStore) GetAllJobs() []JobRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Collect(maps.Values(s.jobs))
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
	removed := 0
	for k, r := range s.executed {
		ts := r.ActionCreatedAt
		if ts == 0 {
			ts = r.ExecutedAt
		}
		if (ts > 0 && ts < cutoff) || (r.ClaimedAt > 0 && r.ClaimedAt < cutoff) {
			delete(s.executed, k)
			removed++
		}
	}
	if removed > 0 {
		log.Debugf("[HelmActions] Cleaned up %d expired action records (remaining: %d)", removed, len(s.executed))
	}

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

// Stop shuts down the cleanup goroutine.
func (s *ActionStore) Stop() {
	close(s.stopCh)
}

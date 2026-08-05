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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	helmactions "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// jobNameLabel is the canonical label kube-controller-manager stamps onto Pods
// owned by a Job (Kubernetes 1.27+). It lets us correlate Pods we observe
// against the JobRecord that OnRollback registered.
const jobNameLabel = "batch.kubernetes.io/job-name"

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
// type ActionStoreInterface interface {
// 	// Claim tries to claim an action for execution. Returns false if already claimed.
// 	Claim(key ActionKey) bool
// 	// MarkExecuted updates the record for a previously claimed action.
// 	MarkExecuted(key ActionKey, status, message string, executedAt, receivedAt, actionCreatedAt int64)
// 	// GetRecord retrieves the execution record for an action.
// 	GetRecord(key ActionKey) (ActionRecord, bool)
// }

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

// PodRecord captures the latest observed state of a Pod owned by a tracked Job.
// Phase reuses corev1.PodPhase directly ("Pending"/"Running"/"Succeeded"/
// "Failed"/"Unknown") so consumers can compare with k8s constants without a
// translation layer.
type PodRecord struct {
	UID         types.UID
	Namespace   string
	Name        string
	JobName     string // value of batch.kubernetes.io/job-name
	Phase       corev1.PodPhase
	Reason      string // e.g. "Error", "OOMKilled", "CrashLoopBackOff"
	Message     string
	ExitCode    int32  // exit code of the helm container (0 if none seen)
	Logs        string // populated lazily when the pod fails
	CreatedAt   int64
	UpdatedAt   int64
	CompletedAt int64
}

// ActionStore tracks processed actions in-memory to prevent duplicate execution.
type ActionStore struct {
	executed map[string]ActionRecord
	jobs     map[types.UID]JobRecord
	pods     map[types.UID]PodRecord
	// mu guards above mentioned
	mu     sync.RWMutex
	stopCh chan struct{}
}

// NewActionStore creates a new ActionStore and starts the background cleanup goroutine.
func NewActionStore() *ActionStore {
	s := &ActionStore{
		executed: make(map[string]ActionRecord),
		jobs:     make(map[types.UID]JobRecord),
		pods:     make(map[types.UID]PodRecord),
	}

	log.Debugf("[HelmActions] Action store initialized (TTL=%v, retention=%v, cleanup=%v)",
		ActionTTL, RecordRetentionTTL, CleanupInterval)
	return s
}

// trackedLifecycle is the union of record types tracked by (namespace, name,
// UID) with a created/updated/completed timestamp lifecycle. Kept as a type
// union rather than an interface because JobRecord and PodRecord otherwise
// share no methods — the shared shape is purely structural.
type trackedLifecycle interface {
	JobRecord | PodRecord
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
	m map[types.UID]T,
	uid types.UID,
	build func(prev T, now int64) (T, bool),
) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := m[uid]
	now := time.Now().Unix()
	rec, transitioned := build(prev, now)
	m[uid] = rec
	return rec, transitioned
}

// TrackJob registers a Job for status tracking. Idempotent: a second call with
// the same UID is a no-op (the watcher will own subsequent updates).
func (s *ActionStore) TrackJob(job *batchv1.Job, _ *helmactions.RollbackInputs) {
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
	return upsertTracked(s, s.jobs, job.UID, func(prev JobRecord, now int64) (JobRecord, bool) {
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
		if prev.CreatedAt == 0 {
			// Watcher saw the Job before OnRollback ran (relisted on reconnect).
			rec.CreatedAt = now
		}
		if rec.CompletedAt == 0 && (phase == JobPhaseSucceeded || phase == JobPhaseFailed) {
			rec.CompletedAt = now
		}
		terminal := rec.CompletedAt > 0 && prev.CompletedAt == 0
		return rec, terminal
	})
}

// RemoveJob drops a tracked Job. Called on watcher DELETED events.
func (s *ActionStore) RemoveJob(uid types.UID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, uid)
}

// UpdatePod applies the latest observed state of a Pod. Returns the resulting
// record and whether this update is the transition into the Failed phase — the
// caller uses that signal to trigger log capture.
func (s *ActionStore) UpdatePod(pod *corev1.Pod) (PodRecord, bool) {
	return upsertTracked(s, s.pods, pod.UID, func(prev PodRecord, now int64) (PodRecord, bool) {
		phase, reason, message, exitCode := classifyPod(pod)
		rec := PodRecord{
			UID:         pod.UID,
			Namespace:   pod.Namespace,
			Name:        pod.Name,
			JobName:     pod.Labels[jobNameLabel],
			Phase:       phase,
			Reason:      reason,
			Message:     message,
			ExitCode:    exitCode,
			Logs:        prev.Logs, // preserve any logs already attached
			CreatedAt:   prev.CreatedAt,
			UpdatedAt:   now,
			CompletedAt: prev.CompletedAt,
		}
		if prev.CreatedAt == 0 {
			rec.CreatedAt = now
		}
		if rec.CompletedAt == 0 && (phase == corev1.PodSucceeded || phase == corev1.PodFailed) {
			rec.CompletedAt = now
		}
		// "Just failed" — the caller uses this edge to fetch logs exactly once.
		justFailed := phase == corev1.PodFailed && prev.Phase != corev1.PodFailed
		return rec, justFailed
	})
}

// RemovePod drops a tracked Pod. Called on watcher DELETED events.
func (s *ActionStore) RemovePod(uid types.UID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pods, uid)
}

// GetPodsForJob returns the tracked Pods whose batch.kubernetes.io/job-name
// label matches the given Job name.
func (s *ActionStore) GetPodsForJob(jobName string) []PodRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []PodRecord
	for _, p := range s.pods {
		if p.JobName == jobName {
			out = append(out, p)
		}
	}
	return out
}

// AttachPodLogs stores the captured tail of a Pod's logs on its record. Safe to
// call when the Pod has already been removed — the update is dropped.
func (s *ActionStore) AttachPodLogs(uid types.UID, logs string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.pods[uid]
	if !ok {
		return
	}
	rec.Logs = logs
	rec.UpdatedAt = time.Now().Unix()
	s.pods[uid] = rec
}

// classifyPod extracts reason/message/exit code from a Pod's status. The exit
// code is taken from the "helm" container; if it has not terminated yet,
// exitCode is 0. The phase is passed through unchanged from pod.Status.Phase.
func classifyPod(pod *corev1.Pod) (corev1.PodPhase, string, string, int32) {
	var (
		reason   = pod.Status.Reason
		message  = pod.Status.Message
		exitCode int32
	)
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name != helmContainerName {
			continue
		}
		if t := cs.State.Terminated; t != nil {
			exitCode = t.ExitCode
			if reason == "" {
				reason = t.Reason
			}
			if message == "" {
				message = t.Message
			}
		} else if w := cs.State.Waiting; w != nil {
			if reason == "" {
				reason = w.Reason
			}
			if message == "" {
				message = w.Message
			}
		}
	}
	return pod.Status.Phase, reason, message, exitCode
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

	removedPods := 0
	for uid, p := range s.pods {
		if p.CompletedAt > 0 && p.CompletedAt < cutoff {
			delete(s.pods, uid)
			removedPods++
		}
	}
	if removedPods > 0 {
		log.Debugf("[HelmActions] Cleaned up %d completed Pod records (remaining: %d)", removedPods, len(s.pods))
	}
}

func (s *ActionStore) RunCleanup(ctx context.Context) {
	s.stopCh = make(chan struct{})
	go s.cleanupLoop(ctx)
}

// Stop shuts down the cleanup goroutine.
func (s *ActionStore) StopCleanup() {
	close(s.stopCh)
	s.stopCh = nil
}

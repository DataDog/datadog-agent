// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package helmactionsimpl

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	jobStuckDurationLimit = 5 * time.Minute
	// TODO: remove in prod before commit
	jobStuckLimitDurationTest = 60 * time.Second
)

func (w *jobWatcher) handleJobEvent(ctx context.Context, ev watch.Event) {
	switch ev.Type {

	case watch.Added, watch.Modified:
		job, ok := ev.Object.(*batchv1.Job)
		if !ok {
			log.Debugf("[HelmActions] Job unexpected object type: %T, ignoring", ev.Object)
			return
		}
		rec, terminal := w.store.UpdateJob(job)

		// check job is reported already

		if terminal {
			log.Infof("[HelmActions] Job %s/%s [%s] reached terminal phase=%s (succeeded=%d failed=%d): %s",
				rec.Namespace, rec.Name, rec.ActionID, rec.Phase, rec.Succeeded, rec.Failed, rec.Message)

			// mark job as reported
			w.reportDone(rec)
			return
		}

		w.reportInprogress(rec)

		log.Infof("[HelmActions] Job %s/%s [%s] reached phase=%s (succeeded=%d failed=%d): %sm conds:%v",
			rec.Namespace, rec.Name, rec.ActionID, rec.Phase, rec.Succeeded, rec.Failed, rec.Message, job.Status.Conditions)

		if !isStuck(job, jobStuckLimitDurationTest) {
			return
		}

		hasFailedCreate, sampleMsg, err := w.hasFailedCreateEvent(ctx, job)
		if err != nil {
			log.Errorf("[HelmActions] error checking events for job %s/%s: %v", job.Namespace, job.Name, err)
			return
		}

		if !hasFailedCreate {
			// Old and idle, but no evidence it's the pod-creation-failure
			// case specifically. Skip it — could just be a slow scheduler,
			// suspended job, etc.
			return
		}

		log.Infof("[HelmActions] stuck job detected: %s/%s [%s] (age=%s) — %s",
			job.Namespace, job.Name, rec.ActionID, time.Since(job.Status.StartTime.Time).Round(time.Second), sampleMsg)

		if err := w.deleteJob(ctx, job); err != nil {
			log.Errorf("[HelmActions] error deleting job %s/%s: %v", job.Namespace, job.Name, err)
		} else {
			log.Infof("[HelmActions] deleted job %s/%s", job.Namespace, job.Name)
			w.reportFailed(rec)
		}

	case watch.Error:
		jobStatus, ok := ev.Object.(*metav1.Status)
		if !ok {
			log.Debugf("[HelmActions] Job %s/%s error, unexpected object type: %T, ignoring", ev.Object)
			return
		}

		log.Infof("[HelmActions] Job error status: %s[%s]: %s", jobStatus.Status, jobStatus.Reason, jobStatus.Message)

	case watch.Deleted:
		job, ok := ev.Object.(*batchv1.Job)
		if !ok {
			log.Debugf("[HelmActions] Job %s/%s deleted, unexpected object type: %T, ignoring", job.Namespace, job.Name, ev.Object)
			return
		}
		w.store.RemoveJob(job.UID)
		log.Debugf("[HelmActions] Job %s/%s deleted, dropped from store", job.Namespace, job.Name)
	}
}

// reportDone emits the terminal action_executed event for a Job that just
// transitioned into a terminal phase (see ActionStore.UpdateJob). rec.ActionID
// and rec.OrgID are carried on the record from TrackJob (ultimately sourced
// from the task that started the rollback, see HelmRollbackHandler.Run) —
// without them the backend has no way to correlate this event back to the
// task/org that requested the rollback.
func (w *jobWatcher) reportDone(rec *JobRecord) {
	if rec.ActionID == "" {
		log.Warnf("[HelmActions] Job %s/%s reached terminal phase=%s but has no ActionID — dropping EVP report",
			rec.Namespace, rec.Name, rec.Phase)
		return
	}

	status := kubeactions.StatusSuccess
	if rec.Phase == JobPhaseFailed {
		status = kubeactions.StatusFailed
	}

	res := kubeactions.ExecutionResult{
		Status:  status,
		Message: rec.Message,
	}
	w.ka.ReportResult(reportFromRecord(rec), res)
}

func (w *jobWatcher) reportFailed(rec *JobRecord) {
	res := kubeactions.ExecutionResult{
		Status:  kubeactions.StatusFailed,
		Message: rec.Message,
	}
	w.ka.ReportResult(reportFromRecord(rec), res)
}

func (w *jobWatcher) reportInprogress(rec *JobRecord) {
	w.ka.ReportProgress(reportFromRecord(rec), rec.Message)
}

func reportFromRecord(rec *JobRecord) kubeactions.ActionReport {
	return kubeactions.ActionReport{
		ActionID:          rec.ActionID,
		ActionType:        kubeactions.ActionTypeHelmRollback,
		OrgID:             rec.OrgID,
		ResourceName:      rec.Release,
		ResourceNamespace: rec.ReleaseNamespace,
	}
}

// isStuck reports whether a Job looks like it's wedged: old enough, and
// showing no signs of any Pod ever having run, succeeded, or failed, and not
// already marked Complete/Failed.
func isStuck(job *batchv1.Job, threshold time.Duration) bool {
	if job.Status.StartTime == nil {
		return false
	}
	if time.Since(job.Status.StartTime.Time) < threshold {
		return false
	}
	if job.Status.Active != 0 || job.Status.Succeeded != 0 || job.Status.Failed != 0 {
		return false
	}
	for _, c := range job.Status.Conditions {
		if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) && c.Status == corev1.ConditionTrue {
			return false
		}
	}
	return true
}

// hasFailedCreateEvent looks for a Warning/FailedCreate event on the given
// Job, which is what the Job controller emits when it can't even create a
// Pod (e.g. missing ServiceAccount).
func (w *jobWatcher) hasFailedCreateEvent(ctx context.Context, job *batchv1.Job) (bool, string, error) {
	fieldSelector := fmt.Sprintf(
		"involvedObject.kind=Job,involvedObject.name=%s,involvedObject.namespace=%s,reason=FailedCreate",
		job.Name, job.Namespace,
	)
	events, err := w.client.CoreV1().Events(job.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return false, "", err
	}
	if len(events.Items) == 0 {
		return false, "", nil
	}
	// Return the most recent message as context for the log line.
	latest := events.Items[0]
	for _, e := range events.Items {
		if e.LastTimestamp.After(latest.LastTimestamp.Time) {
			latest = e
		}
	}
	return true, latest.Message, nil
}

func (w *jobWatcher) deleteJob(ctx context.Context, job *batchv1.Job) error {
	propagation := metav1.DeletePropagationBackground
	return w.client.BatchV1().Jobs(job.Namespace).Delete(ctx, job.Name, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
}

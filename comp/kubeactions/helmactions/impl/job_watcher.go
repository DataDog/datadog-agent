// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package helmactionsimpl

import (
	"context"
	"encoding/json"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (w *jobWatcher) handleJobEvent(ctx context.Context, ev watch.Event) {
	switch ev.Type {

	case watch.Added, watch.Modified:
		job, ok := ev.Object.(*batchv1.Job)
		if !ok {
			log.Debugf("[HelmActions] Job unexpected object type: %T, ignoring", ev.Object)
			return
		}
		rec := w.store.UpdateJob(job)

		if rec.phaseIsTerminal() {
			// check job is reported already
			if !rec.reported() {
				log.Infof("[HelmActions] Job %s/%s [%s] reached terminal phase=%s (succeeded=%d failed=%d): %s",
					rec.Namespace, rec.Name, rec.ActionID, rec.Phase, rec.Succeeded, rec.Failed, rec.Message)
				var info *failureInfo
				if rec.Phase == JobPhaseFailed {
					i, err := w.diagnoseJobFailure(ctx, job)
					if err != nil {
						log.Errorf("[HelmActions] error diagnosing job %s/%s (continue w/o info): %v", job.Namespace, job.Name, err)
					}
					info = i
				}
				w.reportDone(rec, info)
				// mark job as reported
				rec.markReported()
			}
			return
		}

		w.reportInprogress(rec)

		log.Infof("[HelmActions] Job %s/%s [%s] reached phase=%s (succeeded=%d failed=%d): %sm conds:%v",
			rec.Namespace, rec.Name, rec.ActionID, rec.Phase, rec.Succeeded, rec.Failed, rec.Message, job.Status.Conditions)

		if !isStuck(job, jobStuckDurationLimit) {
			return
		}

		failInfo, err := w.maybeFailedCreateEvent(ctx, job)
		if err != nil {
			log.Errorf("[HelmActions] error checking events for job %s/%s: %v", job.Namespace, job.Name, err)
			return
		}

		if failInfo == nil {
			// Old and idle, but no evidence it's the pod-creation-failure
			// case specifically. Skip it — could just be a slow scheduler,
			// suspended job, etc.
			return
		}

		log.Infof("[HelmActions] stuck job detected: %s/%s [%s] (age=%s) — %s",
			job.Namespace, job.Name, rec.ActionID, time.Since(job.Status.StartTime.Time).Round(time.Second), failInfo.Message)

		if err := w.deleteJob(ctx, job); err != nil {
			log.Errorf("[HelmActions] error deleting job %s/%s: %v", job.Namespace, job.Name, err)
		} else {
			log.Infof("[HelmActions] deleted job %s/%s", job.Namespace, job.Name)
			w.reportFailed(rec, failInfo)
			rec.markReported()
		}

	case watch.Error:
		jobStatus, ok := ev.Object.(*metav1.Status)
		if !ok {
			log.Debugf("[HelmActions] Job error, unexpected object type: %T, ignoring", ev.Object)
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
func (w *jobWatcher) reportDone(rec *JobRecord, info *failureInfo) {
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

	// info has higher priority as it is more complete
	if info != nil {
		if info.Message != "" {
			res.Message = info.Message
		}
		bytes, _ := json.Marshal(info)
		log.Debugf("[HelmActions] done, fail info: %q", string(bytes))

		res.Payloads = map[string][]byte{
			"info": bytes,
		}
	}

	w.ka.ReportResult(reportFromRecord(rec), res)
}

func (w *jobWatcher) reportFailed(rec *JobRecord, info *failureInfo) {
	bytes, _ := json.Marshal(info)

	log.Debugf("[HelmActions] fail info: %q", string(bytes))

	res := kubeactions.ExecutionResult{
		Status:  kubeactions.StatusFailed,
		Message: info.Message,
		Payloads: map[string][]byte{
			"info": bytes,
		},
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
		RequestedBy:       "dummy",
		ResourceID:        "dummy", // no ID for HELM
		ResourceKind:      "dummy", // multiple kinds for HELM
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

func (w *jobWatcher) deleteJob(ctx context.Context, job *batchv1.Job) error {
	propagation := metav1.DeletePropagationBackground
	return w.client.BatchV1().Jobs(job.Namespace).Delete(ctx, job.Name, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
}

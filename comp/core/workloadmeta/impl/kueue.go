// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmetaimpl

import (
	"fmt"

	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgerrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kueue"
)

// kueueJob identifies the job owning a Workload. For the plain Pod integration,
// job-uid is the owning pod UID, even when that pod belongs to a RayCluster.
type kueueJob struct {
	namespace string
	uid       string
}

// GetKubernetesKueueWorkloadForPod implements Component#GetKubernetesKueueWorkloadForPod.
func (w *workloadmeta) GetKubernetesKueueWorkloadForPod(pod *wmdef.KubernetesPod) (*wmdef.KubernetesKueueWorkload, error) {
	// Jobs admitted by Kueue can propagate the Workload name to their pods.
	// Prebuilt Workloads and plain Pod groups also provide explicit name references.
	if name := kueue.WorkloadName(pod); name != "" {
		return w.GetKubernetesKueueWorkload(wmdef.GenerateKueueWorkloadEntityID(pod.Namespace, name))
	}

	// This resolver is called for every pod during tag extraction. Only pods
	// managed by the plain Pod integration need the UID-based lookup.
	if !kueue.IsManagedPod(pod) || pod.ID == "" {
		return nil, pkgerrors.NewNotFound("Kueue Workload for pod " + pod.Namespace + "/" + pod.ID)
	}

	// The index and entities are updated together under storeMut. Resolving a
	// pod does not scan the cluster-wide Workload collection.
	w.storeMut.RLock()
	defer w.storeMut.RUnlock()
	workloads := w.kueueWorkloadsByJob[kueueJob{namespace: pod.Namespace, uid: pod.ID}]
	if len(workloads) > 1 {
		return nil, fmt.Errorf("multiple Kueue Workloads for pod %s/%s", pod.Namespace, pod.ID)
	}
	for id := range workloads {
		return w.store[wmdef.KindKubernetesKueueWorkload][id].cached.(*wmdef.KubernetesKueueWorkload), nil
	}
	return nil, pkgerrors.NewNotFound("Kueue Workload for pod " + pod.Namespace + "/" + pod.ID)
}

// updateKueueWorkloadIndex follows changes to the merged Workload, rather than
// individual source events. Caller must hold storeMut for writing.
func (w *workloadmeta) updateKueueWorkloadIndex(previous, current *wmdef.KubernetesKueueWorkload) {
	var oldJob, newJob kueueJob
	if previous != nil {
		oldJob = kueueJob{namespace: previous.Namespace, uid: kueue.JobUID(previous)}
	}
	if current != nil {
		newJob = kueueJob{namespace: current.Namespace, uid: kueue.JobUID(current)}
	}
	if oldJob == newJob {
		return
	}
	if oldJob.uid != "" {
		claims := w.kueueWorkloadsByJob[oldJob]
		delete(claims, previous.ID)
		if len(claims) == 0 {
			delete(w.kueueWorkloadsByJob, oldJob)
		}
	}
	if newJob.uid != "" {
		claims := w.kueueWorkloadsByJob[newJob]
		if claims == nil {
			claims = make(map[string]struct{})
			w.kueueWorkloadsByJob[newJob] = claims
		}
		wasAmbiguous := len(claims) > 1
		claims[current.ID] = struct{}{}
		// Warn at the transition, not every time a pod's tags are recomputed.
		if !wasAmbiguous && len(claims) > 1 {
			w.log.Warnf("Multiple Kueue Workloads claim job %s/%s with their job-uid label; UID-based pod tagging is disabled for this job", newJob.namespace, newJob.uid)
		}
	}
}

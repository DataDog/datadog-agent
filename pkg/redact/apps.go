// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	"encoding/json"

	appsv1 "k8s.io/api/apps/v1"
)

// ScrubDeployment scrubs a deployment.
func ScrubDeployment(d *appsv1.Deployment, scrubber *DataScrubber) {
	ScrubObjectMeta(&d.ObjectMeta, scrubber)
	ScrubPodTemplateSpec(&d.Spec.Template, scrubber)
}

// ScrubDaemonSet scrubs a daemon set.
func ScrubDaemonSet(d *appsv1.DaemonSet, scrubber *DataScrubber) {
	ScrubObjectMeta(&d.ObjectMeta, scrubber)
	ScrubPodTemplateSpec(&d.Spec.Template, scrubber)
}

// ScrubStatefulSet scrubs a stateful set.
func ScrubStatefulSet(s *appsv1.StatefulSet, scrubber *DataScrubber) {
	ScrubObjectMeta(&s.ObjectMeta, scrubber)
	ScrubPodTemplateSpec(&s.Spec.Template, scrubber)
}

// ScrubReplicaSet scrubs a replica set.
func ScrubReplicaSet(r *appsv1.ReplicaSet, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
	ScrubPodTemplateSpec(&r.Spec.Template, scrubber)
}

// ScrubControllerRevision scrubs a controller revision.
func ScrubControllerRevision(r *appsv1.ControllerRevision, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
	if len(r.Data.Raw) == 0 {
		return
	}
	var data interface{}
	if err := json.Unmarshal(r.Data.Raw, &data); err == nil {
		scrubValue(data, scrubber)
		if raw, err := json.Marshal(data); err == nil {
			r.Data.Raw = raw
			return
		}
	}
	r.Data.Raw = []byte(`"` + redactedSecret + `"`)
}

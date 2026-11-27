// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	v1 "k8s.io/api/core/v1"
)

// ScrubEndpoints scrubs the metadata of a Endpoints.
func ScrubEndpoints(r *v1.Endpoints, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubEvent scrubs the metadata of a Event.
func ScrubEvent(r *v1.Event, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubLimitRange scrubs the metadata of a LimitRange.
func ScrubLimitRange(r *v1.LimitRange, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubNamespace scrubs the metadata of a Namespace.
func ScrubNamespace(r *v1.Namespace, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubNode scrubs the metadata of a Node.
func ScrubNode(r *v1.Node, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubPersistentVolume scrubs the metadata of a PersistentVolume.
func ScrubPersistentVolume(r *v1.PersistentVolume, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubPersistentVolumeClaim scrubs the metadata of a PersistentVolumeClaim.
func ScrubPersistentVolumeClaim(r *v1.PersistentVolumeClaim, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubResourceQuota scrubs the metadata of a ResourceQuota.
func ScrubResourceQuota(r *v1.ResourceQuota, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubService scrubs the metadata of a Service.
func ScrubService(r *v1.Service, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubServiceAccount scrubs the metadata of a ServiceAccount.
func ScrubServiceAccount(r *v1.ServiceAccount, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubPodTemplate scrubs a pod template.
func ScrubPodTemplate(t *v1.PodTemplate, scrubber *DataScrubber) {
	ScrubObjectMeta(&t.ObjectMeta, scrubber)
	ScrubPodTemplateSpec(&t.Template, scrubber)
}

// ScrubReplicationController scrubs a replication controller.
func ScrubReplicationController(r *v1.ReplicationController, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
	if r.Spec.Template != nil {
		ScrubPodTemplateSpec(r.Spec.Template, scrubber)
	}
}

// ScrubConfigMap scrubs a config map.
func ScrubConfigMap(c *v1.ConfigMap, scrubber *DataScrubber) {
	ScrubObjectMeta(&c.ObjectMeta, scrubber)
	for key := range c.Data {
		if scrubber.ContainsSensitiveWord(key) {
			c.Data[key] = redactedSecret
		}
	}
	for key := range c.BinaryData {
		if scrubber.ContainsSensitiveWord(key) {
			c.BinaryData[key] = []byte(redactedSecret)
		}
	}
}

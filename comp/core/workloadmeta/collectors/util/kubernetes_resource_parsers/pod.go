// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/apm/instrumentation"
	"github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type podParser struct {
	annotationsFilter []*regexp.Regexp
}

// NewPodParser creates and returns a pod parser based on annotations exclusion list
func NewPodParser(annotationsExclude []string) (ObjectParser, error) {
	filters, err := parseFilters(annotationsExclude)
	if err != nil {
		return nil, err
	}

	return podParser{annotationsFilter: filters}, nil
}

func (p podParser) Parse(obj interface{}) workloadmeta.Entity {
	pod := obj.(*corev1.Pod)
	owners := make([]workloadmeta.KubernetesPodOwner, 0, len(pod.OwnerReferences))
	for _, o := range pod.OwnerReferences {
		owners = append(owners, workloadmeta.KubernetesPodOwner{
			Kind: o.Kind,
			Name: o.Name,
			ID:   string(o.UID),
		})
	}

	var pvcNames []string
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			pvcNames = append(pvcNames, volume.PersistentVolumeClaim.ClaimName)
		}
	}

	var rtcName string
	if pod.Spec.RuntimeClassName != nil {
		rtcName = *pod.Spec.RuntimeClassName
	}

	var gpuVendorList []string
	uniqueGPUVendor := make(map[string]struct{})
	for _, container := range pod.Spec.Containers {
		for resourceName := range container.Resources.Limits {
			gpuName, found := gpu.ExtractSimpleGPUName(gpu.ResourceGPU(resourceName))
			if found {
				uniqueGPUVendor[gpuName] = struct{}{}
			}
		}
	}
	for gpuVendor := range uniqueGPUVendor {
		gpuVendorList = append(gpuVendorList, gpuVendor)
	}

	containersList := make([]workloadmeta.OrchestratorContainer, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		c := workloadmeta.OrchestratorContainer{
			Name: container.Name,
		}
		if cpuReq, found := container.Resources.Requests[corev1.ResourceCPU]; found {
			c.Resources.CPURequest = kubernetes.FormatCPURequests(cpuReq)
		}
		if memoryReq, found := container.Resources.Requests[corev1.ResourceMemory]; found {
			c.Resources.MemoryRequest = kubernetes.FormatMemoryRequests(memoryReq)
		}
		containersList = append(containersList, c)
	}

	return withInstrumentationTags(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   string(pod.UID),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Annotations: filterMapStringKey(pod.Annotations, p.annotationsFilter),
			Labels:      pod.Labels,
		},
		Phase:                      string(pod.Status.Phase),
		Owners:                     owners,
		PersistentVolumeClaimNames: pvcNames,
		Ready:                      isPodReady(pod),
		IP:                         pod.Status.PodIP,
		PriorityClass:              pod.Spec.PriorityClassName,
		QOSClass:                   string(pod.Status.QOSClass),
		RuntimeClass:               rtcName,
		GPUVendorList:              gpuVendorList,
		Containers:                 containersList,
	})
}

// Should be aligned with pkg/util/kubernetes/kubelet/kubelet.go
// (Except the static pods that should go away as fixed long time ago)
func isPodReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// splitMaybeSubscriptedPath checks whether the specified fieldPath is
// subscripted, and
//   - if yes, this function splits the fieldPath into path and subscript, and
//     returns (path, subscript, true).
//   - if no, this function returns (fieldPath, "", false).
//
// Example inputs and outputs:
//
//	"metadata.annotations['myKey']" --> ("metadata.annotations", "myKey", true)
//	"metadata.annotations['a[b]c']" --> ("metadata.annotations", "a[b]c", true)
//	"metadata.labels['']"           --> ("metadata.labels", "", true)
//	"metadata.labels"               --> ("metadata.labels", "", false)
func splitMaybeSubscriptedPath(fieldPath string) (string, string, bool) {
	if !strings.HasSuffix(fieldPath, "']") {
		return fieldPath, "", false
	}
	s := strings.TrimSuffix(fieldPath, "']")
	parts := strings.SplitN(s, "['", 2)
	if len(parts) < 2 {
		return fieldPath, "", false
	}
	if len(parts[0]) == 0 {
		return fieldPath, "", false
	}
	return parts[0], parts[1], true
}

func extractSingleValueFromPodMeta(
	pod *workloadmeta.KubernetesPod,
	c instrumentation.TracerConfig,
) (string, bool, error) {
	if c.ValueFrom == nil {
		log.Debug("tracerConfig.ValueFrom is nil")
		return c.Value, c.Value != "", nil
	}

	if c.ValueFrom.FieldRef == nil {
		log.Debug("tracerConfig.ValueFrom.FieldRef is nil")
		return "", false, nil
	}

	fieldPath := c.ValueFrom.FieldRef.FieldPath
	if path, subscript, ok := splitMaybeSubscriptedPath(fieldPath); ok {
		log.Debugf("found path and subscript: %s | %s", path, subscript)
		switch path {
		case "metadata.annotations":
			value, present := pod.Annotations[subscript]
			return value, present, nil
		case "metadata.labels":
			value, present := pod.Labels[subscript]
			return value, present, nil
		default:
			return "", false, fmt.Errorf("invalid fieldPath with subscript %s", fieldPath)
		}
	}

	log.Debugf("split didn't work for fieldPath %s", fieldPath)

	switch fieldPath {
	case "metadata.name":
		return pod.Name, true, nil
	case "metadata.namespace":
		return pod.Namespace, true, nil
	case "metadata.uid":
		return pod.ID, true, nil
	}

	return "", false, fmt.Errorf("unsupported access of fieldPath %s", fieldPath)
}

func withInstrumentationTags(pod *workloadmeta.KubernetesPod) *workloadmeta.KubernetesPod {
	log.Debug("called withInstrumentationTags")
	targetJSON, ok := pod.Annotations[instrumentation.AppliedTargetAnnotation]
	if !ok {
		log.Debugf("pod doesnt have annotation %s", instrumentation.AppliedTargetAnnotation)
		return pod
	}

	log.Debugf("found instrumentation applied target JSON for pod %s/%s", pod.Namespace, pod.Name)

	var t instrumentation.Target
	if err := json.NewDecoder(strings.NewReader(targetJSON)).Decode(&t); err != nil {
		log.Warnf("error parsing instrumentation target JSON: %s", err)
		return pod
	}

	log.Debugf("found decoded target: %+v", t)

	var (
		isSet      bool
		target     = &workloadmeta.InstrumentationWorkloadTarget{}
		setService = func(v string) { target.Service = v }
		setVersion = func(v string) { target.Version = v }
		setEnv     = func(v string) { target.Env = v }
	)

Loop:
	for _, tc := range t.TracerConfigs {
		var setField func(string)
		switch tc.Name {
		case kubernetes.ServiceTagEnvVar:
			setField = setService
		case kubernetes.VersionTagEnvVar:
			setField = setVersion
		case kubernetes.EnvTagEnvVar:
			setField = setEnv
		default:
			continue Loop
		}

		value, extracted, err := extractSingleValueFromPodMeta(pod, tc)
		if err != nil {
			log.Warnf("Error parsing value workload data from pod metadata for env %s: %s", tc.Name, err)
		} else if extracted {
			setField(value)
			isSet = true
		}
	}

	if isSet {
		pod.EvaluatedInstrumentationWorkloadTarget = target
	}

	return pod
}

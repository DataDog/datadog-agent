// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"

	model "github.com/DataDog/agent-payload/v5/process"

	jsoniter "github.com/json-iterator/go"
	"github.com/twmb/murmur3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	// from https://github.com/kubernetes/kubernetes/blob/abe6321296123aaba8e83978f7d17951ab1b64fd/pkg/util/node/node.go#L43
	nodeUnreachablePodReason = "NodeLost"
)

// ExtractPod returns the protobuf model corresponding to a Kubernetes Pod
// resource.
func ExtractPod(p *corev1.Pod) *model.Pod {
	podModel := model.Pod{
		Metadata: extractMetadata(&p.ObjectMeta),
	}
	// pod spec
	podModel.NodeName = p.Spec.NodeName
	// pod status
	podModel.Phase = string(p.Status.Phase)
	podModel.NominatedNodeName = p.Status.NominatedNodeName
	podModel.IP = p.Status.PodIP
	podModel.RestartCount = 0
	podModel.QOSClass = string(p.Status.QOSClass)
	podModel.PriorityClass = p.Spec.PriorityClassName
	for _, cs := range p.Status.ContainerStatuses {
		podModel.RestartCount += cs.RestartCount
		cStatus := convertContainerStatus(cs)
		podModel.ContainerStatuses = append(podModel.ContainerStatuses, &cStatus)
	}

	for _, cs := range p.Status.InitContainerStatuses {
		podModel.RestartCount += cs.RestartCount
		cStatus := convertContainerStatus(cs)
		podModel.InitContainerStatuses = append(podModel.InitContainerStatuses, &cStatus)
	}
	podModel.Status = computeStatus(p)
	podModel.ConditionMessage = getConditionMessage(p)

	podModel.ResourceRequirements = extractPodResourceRequirements(p.Spec.Containers, p.Spec.InitContainers)

	if len(p.Status.Conditions) > 0 {
		podConditions, conditionTags := extractPodConditions(p)
		podModel.Conditions = podConditions
		podModel.Tags = append(podModel.Tags, conditionTags...)
	}

	if p.Status.StartTime != nil {
		podModel.StartTime = p.Status.StartTime.Unix()
	}
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionTrue {
			podModel.ScheduledTime = c.LastTransitionTime.Unix()
		}
	}

	return &podModel
}

// ExtractPodTemplateResourceRequirements extracts resource requirements of containers and initContainers into model.ResourceRequirements
func ExtractPodTemplateResourceRequirements(template corev1.PodTemplateSpec) []*model.ResourceRequirements {
	return extractPodResourceRequirements(template.Spec.Containers, template.Spec.InitContainers)
}
func extractPodResourceRequirements(containers []corev1.Container, initContainers []corev1.Container) []*model.ResourceRequirements {
	var resReq []*model.ResourceRequirements
	for _, c := range containers {
		if modelReq := convertResourceRequirements(c.Resources, c.Name, model.ResourceRequirementsType_container); modelReq != nil {
			resReq = append(resReq, modelReq)
		}
	}

	for _, c := range initContainers {
		if modelReq := convertResourceRequirements(c.Resources, c.Name, model.ResourceRequirementsType_initContainer); modelReq != nil {
			resReq = append(resReq, modelReq)
		}
	}

	return resReq
}

// GenerateUniqueK8sStaticPodHash is used to create a UID for static pods.
// This should generate a unique id because:
// podName + namespace = unique per host
// podName + namespace + host + clustername = unique
func GenerateUniqueK8sStaticPodHash(host, podName, namespace, clusterName string) string {
	h := fnv.New64()
	_, _ = h.Write([]byte(host))
	_, _ = h.Write([]byte(podName))
	_, _ = h.Write([]byte(namespace))
	_, _ = h.Write([]byte(clusterName))
	return strconv.FormatUint(h.Sum64(), 16)
}

// FillK8sPodResourceVersion is use to set a a custom resource version on a pod
// model.
//
// The resource version field collected from the Kubelet can't be
// trusted because it's not updated, therefore not reflecting changes in
// the pod manifest.
// Compute our own resource version by calculating a hash of the pod
// model content. We'll use this information in place of the Kubelet
// resource version in the payload and for cache interactions.
func FillK8sPodResourceVersion(p *model.Pod) error {
	// Enforce order consistency on slices.
	sort.Strings(p.Metadata.Annotations)
	sort.Strings(p.Metadata.Labels)
	sort.Strings(p.Tags)

	// Marshal the pod message to JSON.
	// We need to enforce order consistency on underlying maps as
	// the standard library does.
	marshaller := jsoniter.ConfigCompatibleWithStandardLibrary
	jsonPodModel, err := marshaller.Marshal(p)
	if err != nil {
		return fmt.Errorf("could not marshal pod model to JSON: %s", err)
	}

	// Replace the payload metadata field with the custom version.
	version := murmur3.Sum64(jsonPodModel)
	p.Metadata.ResourceVersion = fmt.Sprint(version)

	return nil
}

// computeStatus is mostly copied from kubernetes to match what users see in kubectl
// in case of issues, check for changes upstream: https://github.com/kubernetes/kubernetes/blob/1e12d92a5179dbfeb455c79dbf9120c8536e5f9c/pkg/printers/internalversion/printers.go#L685
func computeStatus(p *corev1.Pod) string {
	reason := string(p.Status.Phase)
	if p.Status.Reason != "" {
		reason = p.Status.Reason
	}

	initializing := false
	for i := range p.Status.InitContainerStatuses {
		container := p.Status.InitContainerStatuses[i]
		switch {
		case container.State.Terminated != nil && container.State.Terminated.ExitCode == 0:
			continue
		case container.State.Terminated != nil:
			// initialization is failed
			if len(container.State.Terminated.Reason) == 0 {
				if container.State.Terminated.Signal != 0 {
					reason = fmt.Sprintf("Init:Signal:%d", container.State.Terminated.Signal)
				} else {
					reason = fmt.Sprintf("Init:ExitCode:%d", container.State.Terminated.ExitCode)
				}
			} else {
				reason = "Init:" + container.State.Terminated.Reason
			}
			initializing = true
		case container.State.Waiting != nil && len(container.State.Waiting.Reason) > 0 && container.State.Waiting.Reason != "PodInitializing":
			reason = "Init:" + container.State.Waiting.Reason
			initializing = true
		default:
			reason = fmt.Sprintf("Init:%d/%d", i, len(p.Spec.InitContainers))
			initializing = true
		}
		break
	}
	if !initializing {
		hasRunning := false
		for i := len(p.Status.ContainerStatuses) - 1; i >= 0; i-- {
			container := p.Status.ContainerStatuses[i]

			if container.State.Waiting != nil && container.State.Waiting.Reason != "" {
				reason = container.State.Waiting.Reason
			} else if container.State.Terminated != nil && container.State.Terminated.Reason != "" {
				reason = container.State.Terminated.Reason
			} else if container.State.Terminated != nil && container.State.Terminated.Reason == "" {
				if container.State.Terminated.Signal != 0 {
					reason = fmt.Sprintf("Signal:%d", container.State.Terminated.Signal)
				} else {
					reason = fmt.Sprintf("ExitCode:%d", container.State.Terminated.ExitCode)
				}
			} else if container.Ready && container.State.Running != nil {
				hasRunning = true
			}
		}

		// change pod status back to "Running" if there is at least one container still reporting as "Running" status
		if reason == "Completed" && hasRunning {
			reason = "Running"
		}
	}

	if p.DeletionTimestamp != nil && p.Status.Reason == nodeUnreachablePodReason {
		reason = "Unknown"
	} else if p.DeletionTimestamp != nil {
		reason = "Terminating"
	}

	return reason
}

func convertContainerStatus(cs corev1.ContainerStatus) model.ContainerStatus {
	cStatus := model.ContainerStatus{
		Name:         cs.Name,
		ContainerID:  cs.ContainerID,
		Ready:        cs.Ready,
		RestartCount: cs.RestartCount,
	}
	// detecting the current state
	if cs.State.Waiting != nil {
		cStatus.State = "Waiting"
		cStatus.Message = cs.State.Waiting.Reason + " " + cs.State.Waiting.Message
	} else if cs.State.Running != nil {
		cStatus.State = "Running"
	} else if cs.State.Terminated != nil {
		cStatus.State = "Terminated"
		exitString := "(exit: " + strconv.Itoa(int(cs.State.Terminated.ExitCode)) + ")"
		cStatus.Message = cs.State.Terminated.Reason + " " + cs.State.Terminated.Message + " " + exitString
	}
	return cStatus
}

// convertResourceRequirements converts resource requirements to the payload
// format. Various forms are accepted for resource quantities and this is
// transparently abstracted by Kubernetes APIs.
//
// Documentation: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
func convertResourceRequirements(rq corev1.ResourceRequirements, containerName string, resourceType model.ResourceRequirementsType) *model.ResourceRequirements {
	var (
		limits   = make(map[string]int64)
		requests = make(map[string]int64)
	)

	// mapping between resource lists and payload structures
	quantityHolders := map[*corev1.ResourceList]map[string]int64{
		&rq.Limits:   limits,
		&rq.Requests: requests,
	}

	// mapping between resource names and payload quantity handlers
	quantityHandlers := map[corev1.ResourceName]func(resource.Quantity) int64{
		corev1.ResourceCPU:    func(q resource.Quantity) int64 { return q.MilliValue() },
		corev1.ResourceMemory: func(q resource.Quantity) int64 { return q.Value() },
	}

	for resourceName, handler := range quantityHandlers {
		for resourceList, holder := range quantityHolders {
			if quantity, found := (*resourceList)[resourceName]; found {
				holder[resourceName.String()] = handler(quantity)
			}
		}
	}

	return &model.ResourceRequirements{
		Limits:   limits,
		Requests: requests,
		Name:     containerName,
		Type:     resourceType,
	}
}

// extractPodConditions iterates over pod conditions and returns:
// - the payload representation of those conditions
// - the list of tags that will enable pod filtering by condition
func extractPodConditions(p *corev1.Pod) (conditions []*model.PodCondition, conditionTags []string) {
	conditions = make([]*model.PodCondition, 0, len(p.Status.Conditions))
	conditionTags = make([]string, 0, len(p.Status.Conditions))

	for _, condition := range p.Status.Conditions {
		c := &model.PodCondition{
			Message: condition.Message,
			Reason:  condition.Reason,
			Status:  string(condition.Status),
			Type:    string(condition.Type),
		}
		if !condition.LastProbeTime.IsZero() {
			c.LastProbeTime = condition.LastProbeTime.Unix()
		}
		if !condition.LastTransitionTime.IsZero() {
			c.LastTransitionTime = condition.LastTransitionTime.Unix()
		}

		conditions = append(conditions, c)

		conditionTag := fmt.Sprintf("kube_condition_%s:%s", strings.ToLower(string(condition.Type)), strings.ToLower(string(condition.Status)))
		conditionTags = append(conditionTags, conditionTag)
	}

	return
}

// getConditionMessage loops through the pod conditions, and reports the message of the first one
// (in the normal state transition order) that's doesn't pass
func getConditionMessage(p *corev1.Pod) string {
	messageMap := make(map[corev1.PodConditionType]string)

	// from https://github.com/kubernetes/kubernetes/blob/ddd6d668f6a55cd3a8a2c2f268734e83524e5a7b/staging/src/k8s.io/api/core/v1/types.go#L2439-L2449
	// update if new ones appear
	chronologicalConditions := []corev1.PodConditionType{
		corev1.PodScheduled,
		corev1.PodInitialized,
		corev1.ContainersReady,
		corev1.PodReady,
	}

	// in some cases (eg evicted) we don't have conditions
	// in these cases fall back to status message directly
	if len(p.Status.Conditions) == 0 {
		return p.Status.Message
	}

	// populate messageMap with messages for non-passing conditions
	for _, c := range p.Status.Conditions {
		if c.Status == corev1.ConditionFalse && c.Message != "" {
			messageMap[c.Type] = c.Message
		}
	}

	// return the message of the first one that failed
	for _, c := range chronologicalConditions {
		if m := messageMap[c]; m != "" {
			return m
		}
	}
	return ""
}

// mapToTags converts a map for which both keys and values are strings to a
// slice of strings containing those key-value pairs under the "key:value" form.
// if the map contains empty values we only use the key instead
func mapToTags(m map[string]string) []string {
	slice := make([]string, len(m))

	i := 0
	for k, v := range m {
		// Labels can contain empty values: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
		if v == "" {
			slice[i] = k
		} else {
			slice[i] = k + ":" + v
		}
		i++
	}

	return slice
}

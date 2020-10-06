// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build orchestrator

package orchestrator

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	jsoniter "github.com/json-iterator/go"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/pkg/kubelet/pod"
)

const (
	redactedValue = "********"
	// from https://github.com/kubernetes/kubernetes/blob/abe6321296123aaba8e83978f7d17951ab1b64fd/pkg/util/node/node.go#L43
	nodeUnreachablePodReason = "NodeLost"
)

// ProcessPodlist processes a pod list into process messages
func ProcessPodlist(podList []*v1.Pod, groupID int32, cfg *config.AgentConfig, hostName string, clusterName string, clusterID string, withScrubbing bool, extraTags []string) ([]model.MessageBody, error) {
	start := time.Now()
	podMsgs := make([]*model.Pod, 0, len(podList))

	for p := 0; p < len(podList); p++ {
		// extract pod info
		podModel := extractPodMessage(podList[p])

		// static pods "uid" are actually not unique across nodes.
		// we differ from the k8 uuid format in purpose to differentiate those static pods.
		if pod.IsStaticPod(podList[p]) {
			newUID := generateUniqueStaticPodHash(hostName, podList[p].Name, podList[p].Namespace, clusterName)
			// modify it in the original pod for the YAML and in our model
			podList[p].UID = types.UID(newUID)
			podModel.Metadata.Uid = newUID
		}

		pd := podList[p]
		if SkipKubernetesResource(pd.UID, pd.ResourceVersion, K8sPod) {
			continue
		}

		// insert tagger tags
		tags, err := tagger.Tag(kubelet.PodUIDToTaggerEntityName(string(podList[p].UID)), collectors.HighCardinality)
		if err != nil {
			log.Debugf("Could not retrieve tags for pod: %s", err)
			continue
		}

		// additional tags
		tags = append(tags, fmt.Sprintf("pod_status:%s", strings.ToLower(podModel.Status)))
		tags = append(tags, extraTags...)
		podModel.Tags = tags

		// scrub & generate YAML
		if withScrubbing {
			for c := 0; c < len(podList[p].Spec.Containers); c++ {
				ScrubContainer(&podList[p].Spec.Containers[c], cfg)
			}
			for c := 0; c < len(podList[p].Spec.InitContainers); c++ {
				ScrubContainer(&podList[p].Spec.InitContainers[c], cfg)
			}
		}

		// k8s objects only have json "omitempty" annotations
		// and marshalling is more performant than YAML
		jsonPod, err := jsoniter.Marshal(podList[p])
		if err != nil {
			log.Debugf("Could not marshal pod to JSON: %s", err)
			continue
		}
		podModel.Yaml = jsonPod

		podMsgs = append(podMsgs, podModel)
	}

	groupSize := len(podMsgs) / cfg.MaxPerMessage
	if len(podMsgs)%cfg.MaxPerMessage != 0 {
		groupSize++
	}
	chunked := chunkPods(podMsgs, groupSize, cfg.MaxPerMessage)
	messages := make([]model.MessageBody, 0, groupSize)
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorPod{
			HostName:    hostName,
			ClusterName: clusterName,
			Pods:        chunked[i],
			GroupId:     groupID,
			GroupSize:   int32(groupSize),
			ClusterId:   clusterID,
		})
	}

	log.Debugf("Collected & enriched %d out of %d pods in %s", len(podMsgs), len(podList), time.Now().Sub(start))
	return messages, nil
}

// ScrubContainer scrubs sensitive information in the command line & env vars
func ScrubContainer(c *v1.Container, cfg *config.AgentConfig) {
	// scrub command line
	scrubbedCmd, _ := cfg.Scrubber.ScrubCommand(c.Command)
	c.Command = scrubbedCmd
	// scrub env vars
	for e := 0; e < len(c.Env); e++ {
		// use the "key: value" format to work with the regular credential cleaner
		combination := c.Env[e].Name + ": " + c.Env[e].Value
		scrubbedVal, err := log.CredentialsCleanerBytes([]byte(combination))
		if err == nil && combination != string(scrubbedVal) {
			c.Env[e].Value = redactedValue
		}
	}
}

// chunkPods formats and chunks the pods into a slice of chunks using a specific number of chunks.
func chunkPods(pods []*model.Pod, chunkCount, chunkSize int) [][]*model.Pod {
	chunks := make([][]*model.Pod, 0, chunkCount)

	for c := 1; c <= chunkCount; c++ {
		var (
			chunkStart = chunkSize * (c - 1)
			chunkEnd   = chunkSize * (c)
		)
		// last chunk may be smaller than the chunk size
		if c == chunkCount {
			chunkEnd = len(pods)
		}
		chunks = append(chunks, pods[chunkStart:chunkEnd])
	}

	return chunks
}

// extractPodMessage extracts pod info into the proto model
func extractPodMessage(p *v1.Pod) *model.Pod {
	podModel := model.Pod{
		Metadata: ExtractMetadata(&p.ObjectMeta),
	}
	// pod spec
	podModel.NodeName = p.Spec.NodeName
	// pod status
	podModel.Phase = string(p.Status.Phase)
	podModel.NominatedNodeName = p.Status.NominatedNodeName
	podModel.IP = p.Status.PodIP
	podModel.RestartCount = 0
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
	podModel.Status = ComputeStatus(p)
	podModel.ConditionMessage = GetConditionMessage(p)

	for _, c := range p.Spec.Containers {
		if modelReq := convertResourceRequirements(c.Resources, c.Name, model.ResourceRequirementsType_container); modelReq != nil {
			podModel.ResourceRequirements = append(podModel.ResourceRequirements, modelReq)
		}
	}

	for _, c := range p.Spec.InitContainers {
		if modelReq := convertResourceRequirements(c.Resources, c.Name, model.ResourceRequirementsType_initContainer); modelReq != nil {
			podModel.ResourceRequirements = append(podModel.ResourceRequirements, modelReq)
		}
	}

	return &podModel
}

// resourceRequirements calculations: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#:~:text=Resource%20units%20in%20Kubernetes&text=Limits%20and%20requests%20for%20CPU,A%20Container%20with%20spec.
// CPU: 1/10 of a single core, would represent that as 100m.
// Memory: Memory is measured in bytes. In addition, it may be used with SI suffices (E, P, T, G, M, K, m) or their power-of-two-equivalents (Ei, Pi, Ti, Gi, Mi, Ki).
func convertResourceRequirements(rq v1.ResourceRequirements, containerName string, resourceType model.ResourceRequirementsType) *model.ResourceRequirements {
	requests := map[string]int64{}
	setRequests := false
	setLimits := false
	limits := map[string]int64{}

	cpuLimit := rq.Limits.Cpu()
	if !cpuLimit.IsZero() {
		limits[v1.ResourceCPU.String()] = cpuLimit.MilliValue()
		setLimits = true
	}

	memLimit := rq.Limits.Memory()
	if !memLimit.IsZero() {
		limits[v1.ResourceMemory.String()] = memLimit.Value()
		setLimits = true
	}

	cpuRequest := rq.Requests.Cpu()
	if !cpuRequest.IsZero() {
		requests[v1.ResourceCPU.String()] = cpuRequest.MilliValue()
		setRequests = true
	}

	memRequest := rq.Requests.Memory()
	if !memRequest.IsZero() {
		requests[v1.ResourceMemory.String()] = memRequest.Value()
		setRequests = true
	}

	if !setRequests && !setLimits {
		return nil
	}

	return &model.ResourceRequirements{
		Limits:   limits,
		Requests: requests,
		Name:     containerName,
		Type:     resourceType,
	}
}

func convertContainerStatus(cs v1.ContainerStatus) model.ContainerStatus {
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

// ComputeStatus is mostly copied from kubernetes to match what users see in kubectl
// in case of issues, check for changes upstream: https://github.com/kubernetes/kubernetes/blob/1e12d92a5179dbfeb455c79dbf9120c8536e5f9c/pkg/printers/internalversion/printers.go#L685
func ComputeStatus(p *v1.Pod) string {
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

// GetConditionMessage loops through the pod conditions, and reports the message of the first one
// (in the normal state transition order) that's doesn't pass
func GetConditionMessage(p *v1.Pod) string {
	messageMap := make(map[v1.PodConditionType]string)

	// from https://github.com/kubernetes/kubernetes/blob/ddd6d668f6a55cd3a8a2c2f268734e83524e5a7b/staging/src/k8s.io/api/core/v1/types.go#L2439-L2449
	// update if new ones appear
	chronologicalConditions := []v1.PodConditionType{
		v1.PodScheduled,
		v1.PodInitialized,
		v1.ContainersReady,
		v1.PodReady,
	}

	// populate messageMap with messages for non-passing conditions
	for _, c := range p.Status.Conditions {
		if c.Status == v1.ConditionFalse && c.Message != "" {
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

// this should generate a unique id because:
// podName + namespace = unique per host
// podName + namespace + host + clustername = unique
func generateUniqueStaticPodHash(host, podName, namespace, clusterName string) string {
	h := fnv.New64()
	_, _ = h.Write([]byte(host))
	_, _ = h.Write([]byte(podName))
	_, _ = h.Write([]byte(namespace))
	_, _ = h.Write([]byte(clusterName))
	return strconv.FormatUint(h.Sum64(), 16)
}

// ExtractMetadata extracts standard metadata into the model
func ExtractMetadata(m *metav1.ObjectMeta) *model.Metadata {
	meta := model.Metadata{
		Name:      m.Name,
		Namespace: m.Namespace,
		Uid:       string(m.UID),
	}
	if !m.CreationTimestamp.IsZero() {
		meta.CreationTimestamp = m.CreationTimestamp.Unix()
	}
	if !m.DeletionTimestamp.IsZero() {
		meta.DeletionTimestamp = m.DeletionTimestamp.Unix()
	}
	if len(m.Annotations) > 0 {
		meta.Annotations = make([]string, len(m.Annotations))
		i := 0
		for k, v := range m.Annotations {
			meta.Annotations[i] = k + ":" + v
			i++
		}
	}
	if len(m.Labels) > 0 {
		meta.Labels = make([]string, len(m.Labels))
		i := 0
		for k, v := range m.Labels {
			meta.Labels[i] = k + ":" + v
			i++
		}
	}
	for _, o := range m.OwnerReferences {
		owner := model.OwnerReference{
			Name: o.Name,
			Uid:  string(o.UID),
			Kind: o.Kind,
		}
		meta.OwnerReferences = append(meta.OwnerReferences, &owner)
	}

	return &meta
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build orchestrator

package orchestrator

import (
	"fmt"
	"strconv"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	yaml "gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
)

const (
	redactedValue = "********"
	// from https://github.com/kubernetes/kubernetes/blob/abe6321296123aaba8e83978f7d17951ab1b64fd/pkg/util/node/node.go#L43
	nodeUnreachablePodReason = "NodeLost"
)

// ProcessPodlist processes a pod list into process messages
func ProcessPodlist(podList []*v1.Pod, groupID int32, cfg *config.AgentConfig, hostName string, clusterName string) ([]model.MessageBody, error) {
	start := time.Now()
	podMsgs := make([]*model.Pod, 0, len(podList))

	for p := 0; p < len(podList); p++ {
		// extract pod info
		podModel := extractPodMessage(podList[p])

		// insert tags
		tags, err := tagger.Tag(kubelet.PodUIDToTaggerEntityName(string(podList[p].UID)), collectors.HighCardinality)
		if err != nil {
			log.Debugf("Could not retrieve tags for pod: %s", err)
			continue
		}
		podModel.Tags = tags

		// scrub & generate YAML
		for c := 0; c < len(podList[p].Spec.Containers); c++ {
			scrubContainer(&podList[p].Spec.Containers[c], cfg)
		}
		for c := 0; c < len(podList[p].Spec.InitContainers); c++ {
			scrubContainer(&podList[p].Spec.InitContainers[c], cfg)
		}
		yamlPod, _ := yaml.Marshal(podList[p])
		podModel.Yaml = yamlPod

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
		})
	}

	log.Debugf("Collected & enriched %d pods in %s", len(podMsgs), time.Now().Sub(start))
	return messages, nil
}

// scrubContainer scrubs sensitive information in the command line & env vars
func scrubContainer(c *v1.Container, cfg *config.AgentConfig) {
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
func chunkPods(pods []*model.Pod, chunks, perChunk int) [][]*model.Pod {
	chunked := make([][]*model.Pod, 0, chunks)
	chunk := make([]*model.Pod, 0, perChunk)

	for _, p := range pods {
		chunk = append(chunk, p)
		if len(chunk) == perChunk {
			chunked = append(chunked, chunk)
			chunk = make([]*model.Pod, 0, perChunk)
		}
	}
	if len(chunk) > 0 {
		chunked = append(chunked, chunk)
	}
	return chunked
}

// extractPodMessage extracts pod info into the proto model
func extractPodMessage(p *v1.Pod) *model.Pod {
	// pod medatadata
	podModel := model.Pod{
		Name:      p.Name,
		Namespace: p.Namespace,
		Uid:       string(p.UID),
	}
	if len(p.Annotations) > 0 {
		podModel.Annotations = make([]string, len(p.Annotations))
		i := 0
		for k, v := range p.Annotations {
			podModel.Annotations[i] = k + ":" + v
			i++
		}
	}
	if len(p.Labels) > 0 {
		podModel.Labels = make([]string, len(p.Labels))
		i := 0
		for k, v := range p.Labels {
			podModel.Labels[i] = k + ":" + v
			i++
		}
	}
	for _, o := range p.OwnerReferences {
		owner := model.OwnerReference{
			Name: o.Name,
			Uid:  string(o.UID),
			Kind: o.Kind,
		}
		podModel.OwnerReferences = append(podModel.OwnerReferences, &owner)
	}
	// pod spec
	podModel.NodeName = p.Spec.NodeName
	// pod status
	podModel.Phase = string(p.Status.Phase)
	podModel.NominatedNodeName = p.Status.NominatedNodeName
	podModel.IP = p.Status.PodIP
	if p.Status.StartTime != nil {
		podModel.CreationTimestamp = p.ObjectMeta.CreationTimestamp.Unix()
	}
	podModel.RestartCount = 0
	for _, cs := range p.Status.ContainerStatuses {
		podModel.RestartCount += cs.RestartCount
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
		podModel.ContainerStatuses = append(podModel.ContainerStatuses, &cStatus)
	}

	podModel.Status = ComputeStatus(p)
	podModel.ConditionMessage = GetConditionMessage(p)

	return &podModel
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

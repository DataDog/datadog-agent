// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build orchestrator

package orchestrator

import (
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

const redactedValue = "********"

// ProcessPodlist processes a pod list into process messages
func ProcessPodlist(podList []*v1.Pod, groupID int32, cfg *config.AgentConfig) ([]model.MessageBody, error) {
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
			HostName:    cfg.HostName,
			ClusterName: cfg.KubeClusterName,
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
		podModel.CreationTimestamp = p.Status.StartTime.Unix()
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

	podModel.Status = kubelet.ComputeStatus(p)
	podModel.ConditionMessage = kubelet.GetConditionMessage(p)

	return &podModel
}

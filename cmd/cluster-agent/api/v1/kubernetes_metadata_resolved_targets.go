// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0. This product includes software developed
// at Datadog (https://www.datadoghq.com/). Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"context"
	"slices"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type podResolvedTargetsEntry struct {
	namespace string
	podName   string
	targets   []workloadmeta.KubernetesResolvedTarget
}

type resolvedTargetsSnapshot map[string]podResolvedTargetsEntry

func (srv *KubeMetadataStreamServer) processPodEvent(eventType workloadmeta.EventType, pod *workloadmeta.KubernetesPod) {
	if srv.resolver == nil || pod == nil {
		return
	}

	key := pod.Namespace + "/" + pod.Name
	if eventType == workloadmeta.EventTypeUnset {
		srv.metadataMutex.Lock()
		nodeName := srv.podNodes[key]
		delete(srv.podNodes, key)
		if snapshot := srv.resolvedTargets[nodeName]; snapshot != nil {
			delete(snapshot, key)
		}
		srv.notifyNodeSubscribers(nodeName)
		srv.metadataMutex.Unlock()
		return
	}
	if eventType != workloadmeta.EventTypeSet || pod.NodeName == "" {
		return
	}

	resolved, err := srv.resolver.Resolve(context.Background(), pod)
	if err != nil {
		log.Debugf("Unable to resolve DatadogInstrumentation workload target for pod %s: %v", key, err)
	}

	srv.metadataMutex.Lock()
	defer srv.metadataMutex.Unlock()
	oldNode := srv.podNodes[key]
	if oldNode != "" && oldNode != pod.NodeName {
		delete(srv.resolvedTargets[oldNode], key)
		srv.notifyNodeSubscribers(oldNode)
	}
	srv.podNodes[key] = pod.NodeName
	if srv.resolvedTargets[pod.NodeName] == nil {
		srv.resolvedTargets[pod.NodeName] = make(resolvedTargetsSnapshot)
	}
	srv.resolvedTargets[pod.NodeName][key] = podResolvedTargetsEntry{
		namespace: pod.Namespace,
		podName:   pod.Name,
		targets:   resolved,
	}
	srv.notifyNodeSubscribers(pod.NodeName)
}

func (srv *KubeMetadataStreamServer) notifyNodeSubscribers(nodeName string) {
	for _, ch := range srv.namespaceSubscribers[nodeName] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (srv *KubeMetadataStreamServer) buildResolvedTargetsSnapshot(nodeName string) resolvedTargetsSnapshot {
	srv.metadataMutex.RLock()
	defer srv.metadataMutex.RUnlock()

	snapshot := make(resolvedTargetsSnapshot, len(srv.resolvedTargets[nodeName]))
	for key, entry := range srv.resolvedTargets[nodeName] {
		entry.targets = slices.Clone(entry.targets)
		snapshot[key] = entry
	}
	return snapshot
}

func computeResolvedTargetsDiff(old, current resolvedTargetsSnapshot) []*pb.PodResolvedTargets {
	var diff []*pb.PodResolvedTargets
	for key, entry := range current {
		previous, found := old[key]
		if found && slices.Equal(previous.targets, entry.targets) {
			continue
		}
		diff = append(diff, protoPodResolvedTargets(entry, pb.KubeMetadataEventType_SET))
	}
	for key, entry := range old {
		if _, found := current[key]; !found {
			diff = append(diff, protoPodResolvedTargets(entry, pb.KubeMetadataEventType_UNSET))
		}
	}
	return diff
}

func protoPodResolvedTargets(entry podResolvedTargetsEntry, eventType pb.KubeMetadataEventType) *pb.PodResolvedTargets {
	targets := make([]*pb.ResolvedTarget, 0, len(entry.targets))
	for _, target := range entry.targets {
		targets = append(targets, &pb.ResolvedTarget{
			Group:     target.Group,
			Version:   target.Version,
			Kind:      target.Kind,
			Namespace: target.Namespace,
			Name:      target.Name,
			Uid:       target.ID,
		})
	}
	return &pb.PodResolvedTargets{
		Namespace: entry.namespace,
		PodName:   entry.podName,
		Targets:   targets,
		Type:      eventType,
	}
}

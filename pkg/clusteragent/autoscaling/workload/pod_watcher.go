// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"sync"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	patcherQueueSize = 100
)

// NamespacedPodOwner represents a pod owner in a namespace
type NamespacedPodOwner struct {
	// Namespace is the namespace of the pod owner
	Namespace string
	// Kind is the kind of the pod owner (e.g. Deployment, StatefulSet etc.)
	// ReplicaSet is replaced by Deployment
	Kind string
	// Name is the name of the pod owner
	Name string
}

// PodWatcher indexes pods by their owner
type PodWatcher interface {
	// Run starts the PodWatcher.
	Run(ctx context.Context)
	// GetPodsForOwner returns the pods for the given owner.
	GetPodsForOwner(NamespacedPodOwner) []*workloadmeta.KubernetesPod
	// GetReadyPodsForOwner returns the number of ready pods for the given owner.
	GetReadyPodsForOwner(NamespacedPodOwner) int32
}

// PodWatcherImpl is the implementation of the autoscaling PodWatcher
type PodWatcherImpl struct {
	mutex sync.RWMutex

	wlm                  workloadmeta.Component
	patcher              PodPatcher
	patcherChan          chan *workloadmeta.KubernetesPod
	podsPerPodOwner      map[NamespacedPodOwner]map[string]*workloadmeta.KubernetesPod
	readyPodsPerPodOwner map[NamespacedPodOwner]int32
}

// NewPodWatcher creates a new PodWatcher
func NewPodWatcher(wlm workloadmeta.Component, patcher PodPatcher) *PodWatcherImpl {
	return &PodWatcherImpl{
		wlm:                  wlm,
		patcher:              patcher,
		podsPerPodOwner:      make(map[NamespacedPodOwner]map[string]*workloadmeta.KubernetesPod),
		readyPodsPerPodOwner: make(map[NamespacedPodOwner]int32),
	}
}

// GetPodsForOwner returns the pods for the given owner.
func (pw *PodWatcherImpl) GetPodsForOwner(owner NamespacedPodOwner) []*workloadmeta.KubernetesPod {
	pw.mutex.RLock()
	defer pw.mutex.RUnlock()
	pods, ok := pw.podsPerPodOwner[owner]
	if !ok {
		return nil
	}
	res := make([]*workloadmeta.KubernetesPod, 0, len(pods))
	for _, pod := range pods {
		res = append(res, pod)
	}
	return res
}

// GetReadyPodsForOwner returns the number of ready pods for the given owner.
func (pw *PodWatcherImpl) GetReadyPodsForOwner(owner NamespacedPodOwner) int32 {
	pw.mutex.RLock()
	defer pw.mutex.RUnlock()
	return pw.readyPodsPerPodOwner[owner]
}

// Run subscribes to workloadmeta events and indexes pods by their owner.
func (pw *PodWatcherImpl) Run(ctx context.Context) {
	log.Debug("Starting PodWatcher")

	filter := workloadmeta.NewFilterBuilder().AddKind(workloadmeta.KindKubernetesPod).Build()
	ch := pw.wlm.Subscribe(
		"app-autoscaler-pod-watcher",
		workloadmeta.NormalPriority,
		filter,
	)
	defer pw.wlm.Unsubscribe(ch)

	// Start the goroutine to call the POD patcher
	pw.patcherChan = make(chan *workloadmeta.KubernetesPod, patcherQueueSize)
	defer close(pw.patcherChan)
	go pw.runPatcher(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Debugf("Stopping PodWatcher")
			return
		case eventBundle, more := <-ch:
			eventBundle.Acknowledge()
			if !more {
				log.Debugf("Stopping PodWatcher")
				return
			}
			for _, event := range eventBundle.Events {
				pw.HandleEvent(event)
			}
		}
	}
}

// HandleEvent handles a workloadmeta event and updates the podwatcher state
func (pw *PodWatcherImpl) HandleEvent(event workloadmeta.Event) {
	pw.mutex.Lock()
	defer pw.mutex.Unlock()
	pod, ok := event.Entity.(*workloadmeta.KubernetesPod)
	if !ok {
		log.Debugf("Ignoring event with entity type %T", event.Entity)
		return
	}
	if len(pod.Owners) == 0 {
		log.Debugf("Ignoring pod %s without owner", pod.Name)
		return
	}
	switch event.Type {
	case workloadmeta.EventTypeSet:
		pw.handleSetEvent(pod)
	case workloadmeta.EventTypeUnset:
		pw.handleUnsetEvent(pod)
	default:
		log.Errorf("Ignoring event type %d", event.Type)
	}
}

func (pw *PodWatcherImpl) handleSetEvent(pod *workloadmeta.KubernetesPod) {
	podOwner := getNamespacedPodOwner(pod.Namespace, &pod.Owners[0])
	log.Debugf("Adding pod %s to owner %s", pod.ID, podOwner)
	if _, ok := pw.podsPerPodOwner[podOwner]; !ok {
		pw.podsPerPodOwner[podOwner] = make(map[string]*workloadmeta.KubernetesPod)
	}

	// Update ready pods count
	oldPod, exists := pw.podsPerPodOwner[podOwner][pod.ID]
	if exists && oldPod.Ready && !pod.Ready {
		// Pod was ready and is no longer ready
		pw.readyPodsPerPodOwner[podOwner]--
	} else if (!exists || !oldPod.Ready) && pod.Ready {
		// Pod is new and ready, or was not ready and is now ready
		pw.readyPodsPerPodOwner[podOwner]++
	}

	pw.podsPerPodOwner[podOwner][pod.ID] = pod

	// Write to patcher channel if POD is managed by an autoscaler, just to not pollute queue with non-autoscaler PODs.
	// We don't patcher inline to avoid lagging behind on the workloadmeta events, which would result in inaccurate POD counts.
	if pw.patcher != nil && pw.patcher.shouldObservePod(pod) {
		select {
		case pw.patcherChan <- pod:
		default:
			log.Debugf("Patcher queue is full, skipping pod %s", pod.ID)
		}
	}
}

func (pw *PodWatcherImpl) handleUnsetEvent(pod *workloadmeta.KubernetesPod) {
	podOwner := getNamespacedPodOwner(pod.Namespace, &pod.Owners[0])
	if podOwner.Name == "" {
		log.Debugf("Ignoring pod %s without owner name", pod.Name)
		return
	}
	log.Debugf("Removing pod %s from owner %s", pod.ID, podOwner)
	if _, ok := pw.podsPerPodOwner[podOwner]; !ok {
		return
	}

	// Update ready replicas count if pod was ready
	if pod.Ready {
		pw.readyPodsPerPodOwner[podOwner]--
	}

	delete(pw.podsPerPodOwner[podOwner], pod.ID)
	if len(pw.podsPerPodOwner[podOwner]) == 0 {
		delete(pw.podsPerPodOwner, podOwner)
		delete(pw.readyPodsPerPodOwner, podOwner)
	}
}

func (pw *PodWatcherImpl) runPatcher(ctx context.Context) {
	for {
		pod, more := <-pw.patcherChan
		if !more {
			return
		}

		pw.patcher.observedPodCallback(ctx, pod)
	}
}

func getNamespacedPodOwner(ns string, owner *workloadmeta.KubernetesPodOwner) NamespacedPodOwner {
	res := NamespacedPodOwner{
		Name:      owner.Name,
		Kind:      owner.Kind,
		Namespace: ns,
	}
	if res.Kind == kubernetes.ReplicaSetKind {
		deploymentName := kubernetes.ParseDeploymentForReplicaSet(res.Name)
		if deploymentName != "" {
			res.Kind = kubernetes.DeploymentKind
			res.Name = deploymentName
		}
	}
	return res
}

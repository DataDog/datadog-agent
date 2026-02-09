// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"errors"
	"sync"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	patcherQueueSize = 100
)

var errDeploymentNotValidOwner = errors.New("deployment is not a valid owner")

// NamespacedPodOwner represents a pod owner in a namespace
type NamespacedPodOwner struct {
	// Namespace is the namespace of the pod owner
	Namespace string
	// Kind is the kind of the pod owner (e.g. Deployment, StatefulSet etc.)
	// ReplicaSet is replaced by Deployment
	// Pods with Deployment as direct owner are not included
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

// firstReadyEvent is sent to the firstReadyChan when a pod becomes ready.
type firstReadyEvent struct {
	pod *workloadmeta.KubernetesPod
	// preExisting is true when the pod was already Ready the first time the
	// watcher saw it (i.e., a pod that existed before the agent started).
	preExisting bool
}

// PodWatcherImpl is the implementation of the autoscaling PodWatcher
type PodWatcherImpl struct {
	mutex sync.RWMutex

	wlm                  workloadmeta.Component
	patcher              PodPatcher
	patcherChan          chan *workloadmeta.KubernetesPod
	firstReadyChan       chan firstReadyEvent
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

	// Start the goroutine to annotate pods with their first-ready time
	pw.firstReadyChan = make(chan firstReadyEvent, patcherQueueSize)
	defer close(pw.firstReadyChan)
	go pw.runFirstReadyAnnotator(ctx)

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
	podOwner, err := resolveNamespacedPodOwner(pod)
	if err != nil {
		log.Debugf("Ignoring pod %s with invalid owner %s", pod.ID, err)
		return
	}

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

	if pw.patcher == nil {
		log.Debugf("Pod %s/%s: patcher is nil, skipping patcher and first-ready checks", pod.Namespace, pod.Name)
		return
	}

	// Write to patcher channel if POD is managed by an autoscaler, just to not pollute queue with non-autoscaler PODs.
	// We don't patcher inline to avoid lagging behind on the workloadmeta events, which would result in inaccurate POD counts.
	if pw.patcher.shouldObservePod(pod) {
		select {
		case pw.patcherChan <- pod:
		default:
			log.Debugf("Patcher queue is full, skipping pod %s", pod.ID)
		}
	}

	// If a pod just became ready and doesn't already have the first-ready annotation,
	// queue it for annotation so the KSM check can compute time_to_ready.
	justBecameReady := (!exists || !oldPod.Ready) && pod.Ready
	log.Debugf("Pod %s/%s: exists=%v, pod.Ready=%v, oldPod.Ready=%v, justBecameReady=%v, hasAnnotation=%v",
		pod.Namespace, pod.Name, exists, pod.Ready, exists && oldPod.Ready, justBecameReady,
		pod.Annotations[model.FirstReadyTimeAnnotation] != "")
	if justBecameReady && pod.Annotations[model.FirstReadyTimeAnnotation] == "" {
		// A pod is "pre-existing" if it was already Ready the first time we saw it.
		// This means it existed before the agent started, and its LastTransitionTime
		// may not reflect the original first-ready time.
		preExisting := !exists && pod.Ready
		log.Infof("Pod %s/%s: queueing first-ready annotation (preExisting=%v)", pod.Namespace, pod.Name, preExisting)
		select {
		case pw.firstReadyChan <- firstReadyEvent{pod: pod, preExisting: preExisting}:
		default:
			log.Warnf("First-ready queue is full, skipping pod %s/%s", pod.Namespace, pod.Name)
		}
	}
}

func (pw *PodWatcherImpl) handleUnsetEvent(pod *workloadmeta.KubernetesPod) {
	podOwner, err := resolveNamespacedPodOwner(pod)
	if err != nil {
		log.Debugf("Ignoring pod %s with invalid owner %s", pod.ID, err)
		return
	}

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

func (pw *PodWatcherImpl) runFirstReadyAnnotator(ctx context.Context) {
	for {
		event, more := <-pw.firstReadyChan
		if !more {
			return
		}

		pw.patcher.annotateFirstReady(ctx, event.pod, event.preExisting)
	}
}

func resolveNamespacedPodOwner(pod *workloadmeta.KubernetesPod) (NamespacedPodOwner, error) {
	if len(pod.Owners) == 0 || pod.Owners[0].Kind == kubernetes.DeploymentKind {
		return NamespacedPodOwner{}, errDeploymentNotValidOwner
	}

	res := NamespacedPodOwner{
		Name:      pod.Owners[0].Name,
		Kind:      pod.Owners[0].Kind,
		Namespace: pod.Namespace,
	}

	if res.Kind == kubernetes.ReplicaSetKind {
		// Check if Argo Rollout based on Label
		if pod.Labels != nil && pod.Labels[kubernetes.ArgoRolloutLabelKey] != "" {
			// Note: Argo Rollouts use the same naming convention as Deployments
			rolloutName := kubernetes.ParseDeploymentForReplicaSet(res.Name)
			if rolloutName != "" {
				res.Kind = kubernetes.RolloutKind
				res.Name = rolloutName
			}
		} else {
			// Try to parse as Deployment
			deploymentName := kubernetes.ParseDeploymentForReplicaSet(res.Name)
			if deploymentName != "" {
				res.Kind = kubernetes.DeploymentKind
				res.Name = deploymentName
			}
		}
	}

	return res, nil
}

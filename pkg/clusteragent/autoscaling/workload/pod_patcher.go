// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var podGVR = corev1.SchemeGroupVersion.WithResource("pods")

// PodPatcher allows a workload patcher to patch a workload with the recommendations from the autoscaler
type PodPatcher interface {
	// ApplyRecommendation applies the recommendation to the given pod
	ApplyRecommendations(pod *corev1.Pod) (bool, error)

	// shouldObserverPod returns true if the pod should be observed by the pod watcher
	shouldObservePod(pod *workloadmeta.KubernetesPod) bool

	// observedPodCallback is called when a pod is observed by the pod watcher.
	// It allows to generate events based on the observed pod.
	observedPodCallback(ctx context.Context, pod *workloadmeta.KubernetesPod)
}

type podPatcher struct {
	store         *store
	isLeader      func() bool
	client        dynamic.Interface
	eventRecorder record.EventRecorder
}

var _ PodPatcher = podPatcher{}

func newPODPatcher(store *store, isLeader func() bool, client dynamic.Interface, eventRecorder record.EventRecorder) PodPatcher {
	return podPatcher{
		store:         store,
		isLeader:      isLeader,
		client:        client,
		eventRecorder: eventRecorder,
	}
}

func (pa podPatcher) ApplyRecommendations(pod *corev1.Pod) (bool, error) {
	autoscaler, err := pa.findAutoscaler(pod)
	if err != nil {
		return false, err
	}
	if autoscaler == nil {
		// This POD is not managed by an autoscaler
		return false, nil
	}

	// Check if the autoscaler has recommendations
	if autoscaler.ScalingValues.Vertical == nil || autoscaler.ScalingValues.Vertical.ResourcesHash == "" || len(autoscaler.ScalingValues.Vertical.ContainerResources) == 0 {
		log.Debugf("Autoscaler %s has no vertical recommendations for POD %s/%s, not patching", autoscaler.ID(), pod.Namespace, pod.Name)
		return false, nil
	}

	// Check if we're allowed to patch the POD
	strategy, reason := getVerticalPatchingStrategy(autoscaler)
	if strategy == datadoghq.DatadogPodAutoscalerDisabledUpdateStrategy {
		log.Debugf("Autoscaler %s has vertical patching disabled for POD %s/%s, reason: %s", autoscaler.ID(), pod.Namespace, pod.Name, reason)
		return false, nil
	}

	// Patching the pod with the recommendations
	patched := false
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}

	if pod.Annotations[model.RecommendationIDAnnotation] != autoscaler.ScalingValues.Vertical.ResourcesHash {
		pod.Annotations[model.RecommendationIDAnnotation] = autoscaler.ScalingValues.Vertical.ResourcesHash
		patched = true
	}

	autoscalerID := autoscaler.ID()
	if pod.Annotations[model.AutoscalerIDAnnotation] != autoscalerID {
		pod.Annotations[model.AutoscalerIDAnnotation] = autoscalerID
		patched = true
	}

	// Even if annotation matches, we still verify the resources are correct, in case the POD was modified.
	for _, reco := range autoscaler.ScalingValues.Vertical.ContainerResources {
		for i := range pod.Spec.Containers {
			cont := &pod.Spec.Containers[i]
			if cont.Name != reco.Name {
				continue
			}
			if cont.Resources.Limits == nil {
				cont.Resources.Limits = corev1.ResourceList{}
			}
			if cont.Resources.Requests == nil {
				cont.Resources.Requests = corev1.ResourceList{}
			}
			for resource, limit := range reco.Limits {
				if limit != cont.Resources.Limits[resource] {
					cont.Resources.Limits[resource] = limit
					patched = true
				}
			}
			for resource, request := range reco.Requests {
				if request != cont.Resources.Requests[resource] {
					cont.Resources.Requests[resource] = request
					patched = true
				}
			}
			break
		}
	}

	return patched, nil
}

func (pa podPatcher) findAutoscaler(pod *corev1.Pod) (*model.PodAutoscalerInternal, error) {
	// Pods without owner cannot be autoscaled, ignore it
	if len(pod.OwnerReferences) == 0 {
		return nil, nil
	}

	ownerRef := pod.OwnerReferences[0]
	if ownerRef.Kind == kubernetes.ReplicaSetKind {
		// Check if it's owned by a Deployment, otherwise ReplicaSet is direct owner
		deploymentName := kubernetes.ParseDeploymentForReplicaSet(ownerRef.Name)
		if deploymentName != "" {
			ownerRef.Kind = kubernetes.DeploymentKind
			ownerRef.Name = deploymentName
		}
	}

	// TODO: Implementation is slow
	podAutoscalers := pa.store.GetFiltered(func(podAutoscaler model.PodAutoscalerInternal) bool {
		if podAutoscaler.Namespace == pod.Namespace &&
			podAutoscaler.Spec.TargetRef.Name == ownerRef.Name &&
			podAutoscaler.Spec.TargetRef.Kind == ownerRef.Kind &&
			podAutoscaler.Spec.TargetRef.APIVersion == ownerRef.APIVersion {
			return true
		}
		return false
	})

	if len(podAutoscalers) == 0 {
		return nil, nil
	}

	if len(podAutoscalers) > 1 {
		return nil, log.Errorf("Multiple autoscaler found for POD %s/%s, ownerRef: %s/%s, cannot update POD", pod.Namespace, pod.Name, ownerRef.Kind, ownerRef.Name)
	}

	return &podAutoscalers[0], nil
}

func (pa podPatcher) shouldObservePod(pod *workloadmeta.KubernetesPod) bool {
	return pod.Annotations[model.RecommendationIDAnnotation] != "" &&
		pod.Annotations[model.AutoscalerIDAnnotation] != "" &&
		pod.Annotations[model.RecommendationAppliedEventGeneratedAnnotation] == ""
}

func (pa podPatcher) observedPodCallback(ctx context.Context, pod *workloadmeta.KubernetesPod) {
	if !pa.isLeader() {
		return
	}

	podObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			UID:       types.UID(pod.ID),
		},
	}

	pa.eventRecorder.AnnotatedEventf(podObj,
		map[string]string{"datadog-autoscaler": pod.Annotations[model.AutoscalerIDAnnotation]},
		corev1.EventTypeNormal,
		model.RecommendationAppliedEventReason,
		"POD patched with recommendations from autoscaler %s, recommendation id: %s", pod.Annotations[model.AutoscalerIDAnnotation], pod.Annotations[model.RecommendationIDAnnotation],
	)

	podPatch := []byte(`{"metadata": {"annotations": {"` + model.RecommendationAppliedEventGeneratedAnnotation + `": "true"}}}`)
	_, err := pa.client.Resource(podGVR).Namespace(pod.Namespace).Patch(ctx, pod.Name, types.StrategicMergePatchType, podPatch, metav1.PatchOptions{})
	if err != nil {
		log.Warnf("Failed to patch POD %s/%s with event emitted annotation, event may be generated multiple times, err: %v", pod.Namespace, pod.Name, err)
	}
	log.Debugf("Event sent and POD %s/%s patched with event annotation", pod.Namespace, pod.Name)
}

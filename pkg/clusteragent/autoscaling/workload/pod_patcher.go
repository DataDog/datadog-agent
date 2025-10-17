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

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"

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

// NewPodPatcher creates a new PodPatcher
func NewPodPatcher(store *store, isLeader func() bool, client dynamic.Interface, eventRecorder record.EventRecorder) PodPatcher {
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

	// We're always adding annotation to Pods when a matching Autoscaler is found even if we do not have recommendations ATM
	patched := false
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}

	autoscalerID := autoscaler.ID()
	if pod.Annotations[model.AutoscalerIDAnnotation] != autoscalerID {
		pod.Annotations[model.AutoscalerIDAnnotation] = autoscalerID
		patched = true
	}

	// Check if the autoscaler has recommendations
	if autoscaler.ScalingValues().Vertical == nil || autoscaler.ScalingValues().Vertical.ResourcesHash == "" || len(autoscaler.ScalingValues().Vertical.ContainerResources) == 0 {
		log.Debugf("Autoscaler %s has no vertical recommendations for POD %s/%s, not patching", autoscaler.ID(), pod.Namespace, pod.Name)
		return patched, nil
	}

	// Check if we're allowed to patch the POD
	strategy, reason := getVerticalPatchingStrategy(autoscaler)
	if strategy == datadoghqcommon.DatadogPodAutoscalerDisabledUpdateStrategy {
		log.Debugf("Autoscaler %s has vertical patching disabled for POD %s/%s, reason: %s", autoscaler.ID(), pod.Namespace, pod.Name, reason)
		return patched, nil
	}

	// Patching the pod with the recommendations
	if pod.Annotations[model.RecommendationIDAnnotation] != autoscaler.ScalingValues().Vertical.ResourcesHash {
		pod.Annotations[model.RecommendationIDAnnotation] = autoscaler.ScalingValues().Vertical.ResourcesHash
		patched = true
	}

	// Even if annotation matches, we still verify the resources are correct, in case the POD was modified.
	for _, reco := range autoscaler.ScalingValues().Vertical.ContainerResources {
		patched = patchPod(reco, pod) || patched
	}

	return patched, nil
}

func (pa podPatcher) findAutoscaler(pod *corev1.Pod) (*model.PodAutoscalerInternal, error) {
	// Pods without owner cannot be autoscaled, ignore it
	if len(pod.OwnerReferences) == 0 {
		return nil, nil
	}

	ownerRef := pod.OwnerReferences[0]

	// Ignore pods owned directly by a deployment
	if ownerRef.Kind == kubernetes.DeploymentKind {
		return nil, errDeploymentNotValidOwner
	}

	if ownerRef.Kind == kubernetes.ReplicaSetKind {
		// Check if Argo Rollout based on Label
		if pod.Labels != nil && pod.Labels[kubernetes.ArgoRolloutLabelKey] != "" {
			// Note: Argo Rollouts use the same naming convention as Deployments
			rolloutName := kubernetes.ParseDeploymentForReplicaSet(ownerRef.Name)
			if rolloutName != "" {
				ownerRef.Kind = kubernetes.RolloutKind
				ownerRef.Name = rolloutName
				ownerRef.APIVersion = kubernetes.RolloutAPIVersion
			}
		} else {
			// Check if it's owned by a Deployment, otherwise ReplicaSet is direct owner
			deploymentName := kubernetes.ParseDeploymentForReplicaSet(ownerRef.Name)
			if deploymentName != "" {
				ownerRef.Kind = kubernetes.DeploymentKind
				ownerRef.Name = deploymentName
			}
		}
	}

	// TODO: Implementation is slow
	podAutoscalers := pa.store.GetFiltered(func(podAutoscaler model.PodAutoscalerInternal) bool {
		if podAutoscaler.Namespace() == pod.Namespace &&
			podAutoscaler.Spec().TargetRef.Name == ownerRef.Name &&
			podAutoscaler.Spec().TargetRef.Kind == ownerRef.Kind &&
			podAutoscaler.Spec().TargetRef.APIVersion == ownerRef.APIVersion {
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

// K8s guarantees that the name for an init container or normal container are unique among all containers.
// It means that dispatching recommendations just by container names is sufficient
func patchPod(reco datadoghqcommon.DatadogPodAutoscalerContainerResources, pod *corev1.Pod) (patched bool) {
	for i := range pod.Spec.Containers {
		cont := &pod.Spec.Containers[i]
		if cont.Name == reco.Name {
			return patchContainerResources(reco, cont)
		}
	}

	// recommendation can be also applied to sidecar containers
	// kubernetes implements sidecar containers as a special case of init containers (see https://kubernetes.io/docs/concepts/workloads/pods/sidecar-containers/)
	for i := range pod.Spec.InitContainers {
		cont := &pod.Spec.InitContainers[i]
		// sidecar container by definition is an init container with `restartPolicy: Always`
		isInitSidecarContainer := cont.RestartPolicy != nil && *cont.RestartPolicy == corev1.ContainerRestartPolicyAlways
		if cont.Name == reco.Name && isInitSidecarContainer {
			return patchContainerResources(reco, cont)
		}
	}

	return false
}

func patchContainerResources(reco datadoghqcommon.DatadogPodAutoscalerContainerResources, cont *corev1.Container) (patched bool) {
	patched = false

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
	return patched
}

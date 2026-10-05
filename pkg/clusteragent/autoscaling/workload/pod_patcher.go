// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"sort"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	workloadpatcher "github.com/DataDog/datadog-agent/pkg/clusteragent/patcher"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PodPatcher allows a workload patcher to patch a workload with the recommendations from the autoscaler
type PodPatcher interface {
	// PlanRecommendations evaluates recommendations once without admission writes.
	PlanRecommendations(pod *corev1.Pod) (*PodRecommendationPlan, error)

	// shouldObserverPod returns true if the pod should be observed by the pod watcher
	shouldObservePod(pod *workloadmeta.KubernetesPod) bool

	// observedPodCallback is called when a pod is observed by the pod watcher.
	// It allows to generate events based on the observed pod.
	observedPodCallback(ctx context.Context, pod *workloadmeta.KubernetesPod)
}

type podPatcher struct {
	store         *store
	patcher       *workloadpatcher.Patcher
	eventRecorder record.EventRecorder
}

var _ PodPatcher = podPatcher{}

// NewPodPatcher creates a new PodPatcher
func NewPodPatcher(store *store, patcher *workloadpatcher.Patcher, eventRecorder record.EventRecorder) PodPatcher {
	return podPatcher{
		store:         store,
		patcher:       patcher,
		eventRecorder: eventRecorder,
	}
}

func (pa podPatcher) PlanRecommendations(pod *corev1.Pod) (*PodRecommendationPlan, error) {
	// Sequential recommendations observe earlier planned known-field edits.
	// This private decision view is never serialized as an admission Pod.
	pod = pod.DeepCopy()
	autoscaler, err := pa.findAutoscaler(pod)
	if err != nil {
		return nil, err
	}
	if autoscaler == nil {
		// This POD is not managed by an autoscaler
		return nil, nil
	}

	// We're always adding annotation to Pods when a matching Autoscaler is found even if we do not have recommendations ATM
	plan := &PodRecommendationPlan{}
	plan.annotation(pod, model.AutoscalerIDAnnotation, autoscaler.ID())

	// Check if the autoscaler has recommendations
	if autoscaler.ScalingValues().Vertical == nil || autoscaler.ScalingValues().Vertical.ResourcesHash == "" || len(autoscaler.ScalingValues().Vertical.ContainerResources) == 0 {
		log.Debugf("Autoscaler %s has no vertical recommendations for POD %s/%s, not patching", autoscaler.ID(), pod.Namespace, pod.Name)
		return plan, nil
	}

	// Check if we're allowed to patch the POD
	strategy, reason := getVerticalPatchingStrategy(autoscaler)
	if strategy == datadoghqcommon.DatadogPodAutoscalerDisabledUpdateStrategy {
		log.Debugf("Autoscaler %s has vertical patching disabled for POD %s/%s, reason: %s", autoscaler.ID(), pod.Namespace, pod.Name, reason)
		return plan, nil
	}

	// Re-derive the burstable/constraint transformations here so they are applied consistently on
	// every replica. The controller stamps the removeLimitSentinel only on the leader (and it is
	// stripped from the DPA status), so a follower webhook would otherwise leave the CPU limit in
	// place. Inputs come from the spec/annotations, available on all replicas; idempotent on the leader.
	constrainedVertical := autoscaler.ScalingValues().Vertical.DeepCopy()
	if _, err := applyForcedResources(constrainedVertical, autoscaler.ForcedResources()); err != nil {
		log.Warnf("Autoscaler %s: failed to apply forced resources for POD %s/%s, not patching resources: %v", autoscaler.ID(), pod.Namespace, pod.Name, err)
		return plan, nil
	}
	if _, err := applyVerticalConstraints(constrainedVertical, autoscaler.Spec().Constraints, autoscaler.IsBurstable()); err != nil {
		log.Warnf("Autoscaler %s: failed to apply vertical constraints for POD %s/%s, not patching resources: %v", autoscaler.ID(), pod.Namespace, pod.Name, err)
		return plan, nil
	}

	// Use the active scaling values hash (mirrored to the DPA status) so the annotation stays
	// identical across replicas; not the recomputed constrained hash.
	effectiveRecommendationID := autoscaler.ScalingValues().Vertical.ResourcesHash
	plan.annotation(pod, model.RecommendationIDAnnotation, effectiveRecommendationID)

	// Even if annotation matches, we still verify the resources are correct, in case the POD was modified.
	for _, reco := range constrainedVertical.ContainerResources {
		plan.recommendation(reco, pod)
	}

	runtimeRecID, _ := computeRuntimeRecommendationID(constrainedVertical.ContainerResources)
	plan.annotation(pod, model.RuntimeRecommendationIDAnnotation, runtimeRecID)

	return plan, nil
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
	podAutoscalers := pa.store.List(func(podAutoscaler model.PodAutoscalerInternal) bool {
		return podAutoscaler.Namespace() == pod.Namespace &&
			podAutoscaler.Spec().TargetRef.Name == ownerRef.Name &&
			podAutoscaler.Spec().TargetRef.Kind == ownerRef.Kind &&
			podAutoscaler.Spec().TargetRef.APIVersion == ownerRef.APIVersion &&
			(podAutoscaler.Spec().ApplyPolicy == nil || podAutoscaler.Spec().ApplyPolicy.Mode != datadoghq.DatadogPodAutoscalerApplyModePreview)
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
	intent := workloadpatcher.NewPatchIntent(workloadpatcher.PodTarget(pod.Namespace, pod.Name)).
		With(workloadpatcher.SetMetadataAnnotations(map[string]interface{}{
			model.RecommendationAppliedEventGeneratedAnnotation: "true",
		}))

	applied, err := pa.patcher.Apply(ctx, intent, workloadpatcher.PatchOptions{
		Caller: "autoscaling_pod_patcher",
	})
	if err != nil {
		log.Warnf("Failed to patch POD %s/%s with event emitted annotation, event may be generated multiple times, err: %v", pod.Namespace, pod.Name, err)
		return
	}
	if !applied {
		// Skip: not leader
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

	log.Debugf("Event sent and POD %s/%s patched with event annotation", pod.Namespace, pod.Name)
}

// PodRecommendationPlan holds domain intent independently of admission transport.
type PodRecommendationPlan struct {
	Annotations []RecommendationAnnotation
	Resources   []RecommendationResource
	Runtime     []RecommendationRuntime
}
type RecommendationAnnotation struct {
	Key   string
	Value *string
}
type RecommendationContainer struct {
	Name string
	Init bool
}
type RecommendationResource struct {
	Container RecommendationContainer
	Name      corev1.ResourceName
	Limits    bool
	Quantity  *resource.Quantity
}
type RecommendationRuntime struct {
	Container  RecommendationContainer
	GOMEMLIMIT string
}

// Changed retains the historical feature mutation signal.
func (p *PodRecommendationPlan) Changed() bool {
	return p != nil && len(p.Annotations)+len(p.Resources)+len(p.Runtime) > 0
}

func (p *PodRecommendationPlan) annotation(pod *corev1.Pod, key, value string) {
	old, exists := pod.Annotations[key]
	if value == "" {
		if exists {
			p.Annotations = append(p.Annotations, RecommendationAnnotation{Key: key})
			delete(pod.Annotations, key)
		}
		return
	}
	if old != value {
		p.Annotations = append(p.Annotations, RecommendationAnnotation{key, &value})
		if pod.Annotations == nil {
			pod.Annotations = map[string]string{}
		}
		pod.Annotations[key] = value
	}
}

func (p *PodRecommendationPlan) recommendation(reco datadoghqcommon.DatadogPodAutoscalerContainerResources, pod *corev1.Pod) {
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		if c.Name == reco.Name {
			p.container(reco, c, false)
			return
		}
	}
	for i := range pod.Spec.InitContainers {
		c := &pod.Spec.InitContainers[i]
		if c.Name == reco.Name && c.RestartPolicy != nil && *c.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			p.container(reco, c, true)
			return
		}
	}
}

func (p *PodRecommendationPlan) container(reco datadoghqcommon.DatadogPodAutoscalerContainerResources, cont *corev1.Container, init bool) {
	id := RecommendationContainer{cont.Name, init}
	for _, group := range []struct {
		limits                bool
		recommended, existing corev1.ResourceList
	}{{true, reco.Limits, cont.Resources.Limits}, {false, reco.Requests, cont.Resources.Requests}} {
		names := make([]string, 0, len(group.recommended))
		for name := range group.recommended {
			names = append(names, string(name))
		}
		sort.Strings(names)
		for _, name := range names {
			key := corev1.ResourceName(name)
			quantity := group.recommended[key]
			if group.limits && quantity.Cmp(removeLimitSentinel) == 0 {
				if _, exists := group.existing[key]; exists {
					p.Resources = append(p.Resources, RecommendationResource{Container: id, Name: key, Limits: true})
					delete(cont.Resources.Limits, key)
				}
				continue
			}
			if quantity.Cmp(group.existing[key]) != 0 {
				q := quantity.DeepCopy()
				p.Resources = append(p.Resources, RecommendationResource{Container: id, Name: key, Limits: group.limits, Quantity: &q})
				if group.limits {
					if cont.Resources.Limits == nil {
						cont.Resources.Limits = corev1.ResourceList{}
					}
					cont.Resources.Limits[key] = q
				} else {
					if cont.Resources.Requests == nil {
						cont.Resources.Requests = corev1.ResourceList{}
					}
					cont.Resources.Requests[key] = q
				}
			}
		}
	}
	if reco.Runtime == nil || reco.Runtime.Gomemlimit == "" {
		return
	}
	for i, env := range cont.Env {
		if env.Name == "GOMEMLIMIT" {
			if env.Value != reco.Runtime.Gomemlimit || env.ValueFrom != nil {
				p.Runtime = append(p.Runtime, RecommendationRuntime{id, reco.Runtime.Gomemlimit})
				cont.Env[i].Value, cont.Env[i].ValueFrom = reco.Runtime.Gomemlimit, nil
			}
			return
		}
	}
	p.Runtime = append(p.Runtime, RecommendationRuntime{id, reco.Runtime.Gomemlimit})
	cont.Env = append(cont.Env, corev1.EnvVar{Name: "GOMEMLIMIT", Value: reco.Runtime.Gomemlimit})
}

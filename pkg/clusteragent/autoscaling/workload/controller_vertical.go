// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	k8sutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

const (
	subsystem  = "vertical_controller"
	annotation = "autoscaling.datadoghq.com/scaling-hash"
)

var supportedKinds = map[string]struct{}{
	k8sutil.DeploymentKind: {},
}

// verticalController is responsible for updating targetRef objects with the vertical recommendations
type verticalController struct {
	cl dynamic.Interface
	pw PodWatcher
}

// newVerticalController creates a new *verticalController
func newVerticalController(cl dynamic.Interface, pw PodWatcher) *verticalController {
	res := &verticalController{
		cl: cl,
		pw: pw,
	}
	return res
}

func (u *verticalController) sync(ctx context.Context, autoscalerInternal *model.PodAutoscalerInternal) (processResult, error) {
	// Perform some basic checks on the workload
	if autoscalerInternal.Spec == nil {
		err := fmt.Errorf("missing spec")
		log.Errorf("%s/%s: %s", autoscalerInternal.Namespace, autoscalerInternal.Name, err)
		return withStatusUpdate(true, autoscaling.NoRequeue), err
	}

	if autoscalerInternal.Spec.TargetRef.Name == "" {
		err := fmt.Errorf("missing target ref name")
		log.Errorf("%s/%s: %s", autoscalerInternal.Namespace, autoscalerInternal.Name, err)
		return withStatusUpdate(true, autoscaling.NoRequeue), err
	}

	if autoscalerInternal.Spec.TargetRef.Kind == "" {
		err := fmt.Errorf("missing target ref kind")
		log.Errorf("%s/%s: %s", autoscalerInternal.Namespace, autoscalerInternal.Name, err)
		return withStatusUpdate(true, autoscaling.NoRequeue), err
	}

	if autoscalerInternal.ScalingValues.Vertical == nil || len(autoscalerInternal.ScalingValues.Vertical.ContainerResources) == 0 {
		log.Debugf("%s/%s: No vertical recommendations", autoscalerInternal.Namespace, autoscalerInternal.Name)
		return withStatusUpdate(false, autoscaling.NoRequeue), nil
	}

	if autoscalerInternal.Generation == 0 {
		err := fmt.Errorf("no scaling generation for workload")
		log.Errorf("%s/%s: %s", autoscalerInternal.Namespace, autoscalerInternal.Name, err)
		return withStatusUpdate(true, autoscaling.NoRequeue), err
	}

	gvk, err := autoscalerInternal.GetTargetGVK()
	if err != nil || autoscalerInternal.Spec.TargetRef.APIVersion == "" {
		err := fmt.Errorf("failed to parse API version %s: %w", autoscalerInternal.Spec.TargetRef.APIVersion, err)
		log.Errorf("%s/%s: %s", autoscalerInternal.Namespace, autoscalerInternal.Name, err)
		return withStatusUpdate(true, autoscaling.NoRequeue), err
	}

	// Check if the owner kind is supported
	kind := autoscalerInternal.Spec.TargetRef.Kind
	if _, ok := supportedKinds[kind]; !ok {
		err := fmt.Errorf("Unsupported owner kind %s", kind)
		log.Errorf("%s/%s: %s", autoscalerInternal.Namespace, autoscalerInternal.Name, err)
		return withStatusUpdate(true, autoscaling.NoRequeue), err
	}

	// Get the pod owner from the workload
	target := NamespacedPodOwner{
		Namespace: autoscalerInternal.Namespace,
		Name:      autoscalerInternal.Spec.TargetRef.Name,
		Kind:      kind,
	}

	// Get the pods for the pod owner
	pods := u.pw.GetPodsForOwner(target)
	if len(pods) == 0 {
		err := fmt.Errorf("Failed to get pods for owner %s", target)
		log.Debugf("%s/%s: %s", autoscalerInternal.Namespace, autoscalerInternal.Name, err)
		return withStatusUpdate(true, autoscaling.Requeue), err
	}

	// Check if a pod needs to be updated
	needUpdate := false
	recommendations := autoscalerInternal.ScalingValues.Vertical.ContainerResources
	for _, pod := range pods {
		for _, reco := range recommendations {
			for _, cont := range pod.Containers {
				if !equalResources(reco.Requests, cont.Requests) ||
					!equalResources(reco.Limits, cont.Limits) {
					needUpdate = true
					break
				}
			}
		}
	}
	if !needUpdate {
		log.Debugf("%s/%s: No update needed for workload", autoscalerInternal.Namespace, autoscalerInternal.Name)
		return withStatusUpdate(false, autoscaling.NoRequeue), nil
	}

	// Check if a rollout is ongoing
	if target.Kind == k8sutil.DeploymentKind && rolloutOngoingForDeployment(pods) {
		err := fmt.Errorf("another rollout is ongoing")
		log.Debugf("%s/%s: %s", autoscalerInternal.Namespace, autoscalerInternal.Name, err)
		return withStatusUpdate(true, autoscaling.Requeue), err
	}

	// Generate the patch request which adds the scaling hash annotation to the pod template
	gvr := gvk.GroupVersion().WithResource(fmt.Sprintf("%ss", strings.ToLower(gvk.Kind)))
	patchData, err := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]string{
						annotation: strconv.FormatInt(autoscalerInternal.Generation, 16),
					},
				},
			},
		},
	})
	if err != nil {
		err := fmt.Errorf("Failed to serialize data :%v", patchData)
		log.Errorf("%s/%s: %s", autoscalerInternal.Namespace, autoscalerInternal.Name, err)
		return withStatusUpdate(true, autoscaling.NoRequeue), err
	}

	// Patch the target
	_, err = u.cl.Resource(gvr).Namespace(target.Namespace).Patch(ctx, target.Name, types.MergePatchType, patchData, metav1.PatchOptions{})
	if err != nil {
		err := fmt.Errorf("Failed to patch %s: %s/%s -- %v", gvr.String(), target.Namespace, target.Name, err)
		log.Errorf("%s/%s: %s", autoscalerInternal.Namespace, autoscalerInternal.Name, err)
		Patches.Inc(target.Kind, target.Name, target.Namespace, "error")
		return withStatusUpdate(true, autoscaling.Requeue), err

	}
	Patches.Inc(target.Kind, target.Name, target.Namespace, "success")
	return withStatusUpdate(true, autoscaling.Requeue), nil
}

func equalResources(resourceList corev1.ResourceList, resourceMap map[string]string) bool {
	if len(resourceList) != len(resourceMap) {
		return false
	}
	for k, v := range resourceList {
		res, ok := resourceMap[string(k)]
		if !ok {
			return false
		}
		quantity, err := resource.ParseQuantity(res)
		if err != nil {
			log.Errorf("Failed to parse quantity %s", res)
			return false
		}
		if !v.Equal(quantity) {
			return false
		}
	}
	return true
}

// rolloutOngoingForDeployment returns true if a rollout is ongoing for the given deployment
// It compares the owner of the pods to check if they are the same.
// If not, it means that a replicaset has been created and a rollout is ongoing.
func rolloutOngoingForDeployment(pods []*workloadmeta.KubernetesPod) bool {
	// Check if a rollout is already in progress
	var ownerID string
	for _, pod := range pods {
		if len(pod.Owners) == 0 {
			// This condition should never happen since the pod watcher groups pods by owner
			log.Warnf("Pod %s/%s has no owner", pod.Namespace, pod.Name)
			continue
		}

		if ownerID == "" {
			ownerID = pod.Owners[0].ID
		} else if ownerID != pod.Owners[0].ID {
			log.Debugf("Pod %s/%s has owner different from %s (%s), a rollout might be ongoing. Retrying later.", pod.Namespace, pod.Name, ownerID, pod.Owners[0].ID)
			return true
		}
	}
	return false
}

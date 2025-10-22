// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

type store = autoscaling.Store[minNodePool]

const datadogCreatedLabelKey = "datadoghq.com/datadog-cluster-autoscaler.created"
const datadogModifiedLabelKey = "datadoghq.com/datadog-cluster-autoscaler.modified"

var (
	nodePoolGVR = schema.GroupVersionResource{Group: "karpenter.sh", Version: "v1", Resource: "nodepools"}
	// Only support EC2 for now
	nodeClassGVR = schema.GroupVersionResource{Group: "karpenter.k8s.aws", Version: "v1", Resource: "ec2nodeclasses"}

	controllerID = "dca-c"
)

type Controller struct {
	*autoscaling.Controller

	clusterID     string
	clock         clock.Clock
	context       context.Context
	eventRecorder record.EventRecorder
	rcClient      RcClient
	store         *store
	localSender   sender.Sender
}

// NewController returns a new cluster autoscaling controller
func NewController(
	clock clock.Clock,
	clusterID string,
	eventRecorder record.EventRecorder,
	rcClient RcClient,
	dynamicClient dynamic.Interface,
	dynamicInformer dynamicinformer.DynamicSharedInformerFactory,
	isLeaderFunc func() bool,
	store *store,
	localSender sender.Sender,
) (*Controller, error) {

	c := &Controller{
		clusterID:     clusterID,
		clock:         clock,
		eventRecorder: eventRecorder,
		rcClient:      rcClient,
		localSender:   localSender,
	}

	autoscalingWorkqueue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedItemBasedRateLimiter[string](),
		workqueue.TypedRateLimitingQueueConfig[string]{
			Name:            subsystem,
			MetricsProvider: autoscalingQueueMetricsProvider,
		},
	)

	baseController, err := autoscaling.NewController(controllerID, c, dynamicClient, dynamicInformer, nodePoolGVR, isLeaderFunc, store, autoscalingWorkqueue)
	if err != nil {
		return nil, err
	}

	c.Controller = baseController

	// TODO add later, if needed, when adding more telemetry
	// store.RegisterObserver(autoscaling.Observer{
	// 	DeleteFunc: unsetTelemetry,
	// })

	c.store = store

	return c, nil
}

// PreStart is called before the controller starts
func (c *Controller) PreStart(ctx context.Context) {
	startLocalTelemetry(ctx, c.localSender, []string{"kube_cluster_id:" + c.clusterID})
}

// Process implements the Processor interface (so required to be public)
// this processes what's in the workqueue, comes from the store or cluster
func (c *Controller) Process(ctx context.Context, _, ns, name string) autoscaling.ProcessResult {
	// Follower should not process workqueue items
	if !c.IsLeader() {
		return autoscaling.ProcessResult{}
	}

	// Try to get NodePool from cluster
	np := &karpenterv1.NodePool{}
	npUnstr, err := c.Lister.Get(name)
	if err == nil {
		err = autoscaling.FromUnstructured(npUnstr, np)
	}

	switch {
	case apierrors.IsNotFound(err):
		// Ignore not found error as it will be created later
		np = nil
	case err != nil:
		log.Errorf("Unable to retrieve NodePool: %w", err)
		return autoscaling.Requeue
	case npUnstr == nil:
		log.Errorf("Could not parse empty NodePool from local cache")
		return autoscaling.Requeue
	}

	// TODO create duplicate NodePools with greater weight, rather than updating user NodePools
	mnp, foundInStore := c.store.LockRead(name, true)

	if foundInStore {
		if np == nil {
			// Present in store but not found in cluster; create it
			if err = c.createNodePool(ctx, mnp); err != nil {
				log.Errorf("Error creating NodePool: %v", err)
			}
		} else {
			// Present in store and found in cluster; update it
			// TODO check if hash of spec from remote config matches current object before updating
			if err = c.patchNodePool(ctx, np, mnp); err != nil {
				log.Errorf("Error updating NodePool: %v", err)
			}
		}
	} else {
		if isCreatedByDatadog(np.GetLabels()) {
			// Not present in store, and the cluster NodePool is fully managed, then delete the NodePool
			if err = c.deleteNodePool(ctx, name); err != nil {
				log.Errorf("Error deleting NodePool: %v", err)
			}
		} else {
			// Not present in store and the cluster NodePool is not fully managed, do nothing
			log.Debugf("NodePool %s not found in store and is not fully managed, nothing to do", name)
		}
	}

	c.store.Unlock(name)

	return autoscaling.ProcessResult{}
}

func (c *Controller) createNodePool(ctx context.Context, mnp minNodePool) error {
	log.Infof("Creating NodePool: %s", mnp.name)

	// Get NodeClass. If there's none or more than one, then we should not create the NodePool
	ncList, err := c.Client.Resource(nodeClassGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("unable to list NodeClasses: %w", err)
	}

	if len(ncList.Items) == 0 {
		return fmt.Errorf("no NodeClasses found, NodePool cannot be created")
	}

	if len(ncList.Items) > 1 {
		return fmt.Errorf("too many NodeClasses found (%v), NodePool cannot be created", len(ncList.Items))
	}

	u := ncList.Items[0]

	jsonBytes, err := json.Marshal(u.Object)
	if err != nil {
		return fmt.Errorf("unable to marshal unstructured object: %v\n", err)
	}

	var nc minNodeClass
	err = json.Unmarshal(jsonBytes, &nc)
	if err != nil {
		return fmt.Errorf("unable to unmarshal into minNodeClass: %v\n", err)
	}

	nodeClassName := nc.Metadata.Name

	npObj := &karpenterv1.NodePool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodePool",
			APIVersion: "karpenter.sh/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   mnp.name,
			Labels: map[string]string{datadogCreatedLabelKey: "true"},
		},
		Spec: buildNodePoolSpec(mnp, nodeClassName),
	}

	npUnstr, err := autoscaling.ToUnstructured(npObj)
	if err != nil {
		return err
	}

	_, err = c.Client.Resource(nodePoolGVR).Create(ctx, npUnstr, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("unable to create NodePool: %s, err: %v", mnp.name, err)
	}

	return nil
}

func (c *Controller) patchNodePool(ctx context.Context, np *karpenterv1.NodePool, mnp minNodePool) error {
	log.Infof("Patching NodePool: %s", mnp.name)

	patchData := buildNodePoolPatch(np, mnp)
	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		return fmt.Errorf("error marshaling patch data: %s, err: %v", mnp.name, err)
	}

	// TODO: If NodePool is not considered a custom resource in the future, use StrategicMergePatchType
	_, err = c.Client.Resource(nodePoolGVR).Patch(ctx, mnp.name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("unable to update NodePool: %s, err: %v", mnp.name, err)
	}

	return nil
}

func (c *Controller) deleteNodePool(ctx context.Context, name string) error {
	log.Infof("Deleting NodePool: %s", name)

	err := c.Client.Resource(nodePoolGVR).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("Unable to delete NodePool: %s, err: %v", name, err)
	}

	return nil
}

// buildNodePoolSpec is used for creating new NodePools
func buildNodePoolSpec(mnp minNodePool, nodeClassName string) karpenterv1.NodePoolSpec {

	// Convert domain labels into requirements
	reqs := []karpenterv1.NodeSelectorRequirementWithMinValues{}
	for k, v := range mnp.labels {
		reqs = append(reqs, karpenterv1.NodeSelectorRequirementWithMinValues{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      k,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{v},
			},
		})
	}

	// Convert instance types into a requirement
	reqs = append(reqs, karpenterv1.NodeSelectorRequirementWithMinValues{
		NodeSelectorRequirement: corev1.NodeSelectorRequirement{
			Key:      corev1.LabelInstanceTypeStable,
			Operator: corev1.NodeSelectorOpIn,
			Values:   mnp.recommendedInstanceTypes,
		},
	})

	return karpenterv1.NodePoolSpec{
		Template: karpenterv1.NodeClaimTemplate{
			Spec: karpenterv1.NodeClaimTemplateSpec{
				// Include taints
				Taints:       mnp.taints,
				Requirements: reqs,
				NodeClassRef: &karpenterv1.NodeClassReference{
					// Only support EC2NodeClass for now
					Kind:  "EC2NodeClass",
					Name:  nodeClassName,
					Group: "karpenter.k8s.aws",
				},
			},
		},
	}
}

// buildNodePoolPatch is used to construct JSON patch
func buildNodePoolPatch(np *karpenterv1.NodePool, mnp minNodePool) map[string]interface{} {

	// Build requirements patch, only updating values for the instance types
	updatedRequirements := []map[string]interface{}{}
	instanceTypeLabelExists := false
	for _, r := range np.Spec.Template.Spec.Requirements {
		if r.Key == corev1.LabelInstanceTypeStable {
			instanceTypeLabelExists = true
			r.Operator = "In"
			r.Values = mnp.recommendedInstanceTypes
		}

		updatedRequirements = append(updatedRequirements, map[string]interface{}{
			"key":      r.Key,
			"operator": string(r.Operator),
			"values":   r.Values,
		})
	}

	if !instanceTypeLabelExists {
		updatedRequirements = append(updatedRequirements, map[string]interface{}{
			"key":      corev1.LabelInstanceTypeStable,
			"operator": "In",
			"values":   mnp.recommendedInstanceTypes,
		})
	}

	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				datadogModifiedLabelKey: "true",
			},
		},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"requirements": updatedRequirements,
				},
			},
		},
	}
}

func isCreatedByDatadog(labels map[string]string) bool {
	if _, ok := labels[datadogCreatedLabelKey]; ok {
		return true
	}
	return false
}

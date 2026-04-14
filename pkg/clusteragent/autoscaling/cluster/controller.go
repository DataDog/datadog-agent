// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	jsonpatch "github.com/evanphx/json-patch/v5"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type store = autoscaling.Store[model.NodePoolInternal]

var (
	nodePoolGVR = schema.GroupVersionResource{Group: "karpenter.sh", Version: "v1", Resource: "nodepools"}

	// Supported node class GVRs, tried in order during discovery
	ec2NodeClassGVR = schema.GroupVersionResource{Group: "karpenter.k8s.aws", Version: "v1", Resource: "ec2nodeclasses"}
	eksNodeClassGVR = schema.GroupVersionResource{Group: "eks.amazonaws.com", Version: "v1", Resource: "nodeclasses"}

	// nodeClassGVRByGroup maps a NodeClassReference Group to its GVR
	nodeClassGVRByGroup = map[string]schema.GroupVersionResource{
		ec2NodeClassGVR.Group: ec2NodeClassGVR,
		eksNodeClassGVR.Group: eksNodeClassGVR,
	}

	controllerID autoscaling.SenderID = "dca-c"
)

type Controller struct {
	*autoscaling.Controller

	clusterID     string
	clock         clock.Clock
	eventRecorder record.EventRecorder
	rcClient      RcClient
	store         *store
	storeUpdated  *bool
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
	storeUpdated *bool,
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
	c.storeUpdated = storeUpdated

	return c, nil
}

// PreStart is called before the controller starts
func (c *Controller) PreStart(ctx context.Context) {
	autoscaling.StartLocalTelemetry(ctx, c.localSender, "cluster", []string{"orch_cluster_id:" + c.clusterID})
}

// Process implements the Processor interface (so required to be public)
// this processes what's in the workqueue, comes from the store or cluster
func (c *Controller) Process(ctx context.Context, _, _, name string) autoscaling.ProcessResult {
	if !c.IsLeader() || !*c.storeUpdated {
		// Requeue in case of a delay in leader election or the store being updated
		return autoscaling.Requeue
	}

	// Try to get Datadog-managed NodePool from cluster
	datadogNp := &karpenterv1.NodePool{}
	npUnstr, err := c.Lister.Get(name)
	if err == nil {
		err = autoscaling.FromUnstructured(npUnstr, datadogNp)
	}

	switch {
	case apierrors.IsNotFound(err):
		// Ignore not found error as it will be created later
		datadogNp = nil
	case err != nil:
		log.Errorf("Unable to retrieve NodePool: %v", err)
		return autoscaling.Requeue
	case npUnstr == nil:
		log.Errorf("Could not parse empty NodePool from local cache")
		return autoscaling.Requeue
	}

	return c.syncNodePool(ctx, name, datadogNp)
}

func (c *Controller) syncNodePool(ctx context.Context, name string, datadogNp *karpenterv1.NodePool) autoscaling.ProcessResult {
	npi, foundInStore, unlock := c.store.LockRead(name, true)
	defer unlock()

	if foundInStore {
		// Get Target NodePool from Lister if needed
		var targetNp *karpenterv1.NodePool
		if npi.TargetName() != "" {
			targetNp = &karpenterv1.NodePool{}
			targetNpUnstr, err := c.Lister.Get(npi.TargetName())
			if err != nil {
				log.Errorf("Error retrieving Target NodePool: %v", err)
				return autoscaling.Requeue
			}
			err = autoscaling.FromUnstructured(targetNpUnstr, targetNp)
			if err != nil {
				log.Errorf("Error converting Target NodePool: %v", err)
				return autoscaling.Requeue
			}

			// Only create or update if the TargetHash has not changed
			if npi.TargetHash() != targetNp.GetAnnotations()[model.KarpenterNodePoolHashAnnotationKey] {
				log.Infof("NodePool: %s TargetHash (%s) has changed since recommendation was generated; no action will be applied.", npi.Name(), npi.TargetHash())
				return autoscaling.NoRequeue
			}
		}

		if datadogNp == nil {
			// Present in store but not found in cluster; create it
			if err := c.createNodePool(ctx, targetNp, npi); err != nil {
				log.Errorf("Error creating NodePool: %v", err)
				return autoscaling.Requeue
			}
		} else {
			// Present in store and found in cluster; update it
			if err := c.updateNodePool(ctx, targetNp, datadogNp, npi); err != nil {
				log.Errorf("Error updating NodePool: %v", err)
				return autoscaling.Requeue
			}
		}
	} else {
		if datadogNp != nil && isCreatedByDatadog(datadogNp.GetLabels()) {
			// Not present in store, and the cluster NodePool is fully managed, then delete the NodePool
			if err := c.deleteNodePool(ctx, name, datadogNp); err != nil {
				log.Errorf("Error deleting NodePool: %v", err)
				return autoscaling.Requeue
			}
		} else {
			// Not present in store and the cluster NodePool is not fully managed, do nothing
			log.Debugf("NodePool %s not found in store and is not fully managed, nothing to do", name)
		}
	}

	return autoscaling.NoRequeue
}

func (c *Controller) createNodePool(ctx context.Context, targetNp *karpenterv1.NodePool, npi model.NodePoolInternal) error {
	log.Infof("Creating NodePool: %s", npi.Name())

	// Use manifest-provided NodePool when available
	if knp := npi.KarpenterNodePool(); knp != nil {
		knp = knp.DeepCopy()
		// If the manifest omits NodeClassRef and a target NodePool exists, prefer its NodeClassRef
		if knp.Spec.Template.Spec.NodeClassRef == nil && targetNp != nil {
			knp.Spec.Template.Spec.NodeClassRef = targetNp.Spec.Template.Spec.NodeClassRef.DeepCopy()
		}
		var err error
		knp, err = c.checkValidNodeClass(ctx, knp)
		if err != nil {
			return fmt.Errorf("unable to update NodePool with node class: %s, err: %v", npi.Name(), err)
		}
		// Update the weight if replica NodePool
		if knp.Spec.Weight == nil && targetNp != nil {
			knp.Spec.Weight = model.GetNodePoolWeight(targetNp)
		}
		// Ensure Datadog autoscaling node label is always present
		if knp.Spec.Template.ObjectMeta.Labels == nil {
			knp.Spec.Template.ObjectMeta.Labels = make(map[string]string)
		}
		knp.Spec.Template.ObjectMeta.Labels[kubernetes.AutoscalingLabelKey] = "true"
		// add Datadog labels and annotations
		if knp.Labels == nil {
			knp.Labels = make(map[string]string)
		}
		knp.Labels[model.DatadogCreatedLabelKey] = "true"
		if knp.Annotations == nil {
			knp.Annotations = make(map[string]string)
		}
		if npi.TargetName() != "" {
			knp.Annotations[model.DatadogReplicaAnnotationKey] = npi.TargetName()
		}

		npUnstr, err := convertNodePoolToUnstructured(knp)
		if err != nil {
			return fmt.Errorf("unable to convert NodePool to unstructured: %s, err: %v", npi.Name(), err)
		}
		_, err = c.Client.Resource(nodePoolGVR).Create(ctx, npUnstr, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("unable to create NodePool: %s, err: %v", npi.Name(), err)
		}
		c.eventRecorder.Eventf(knp, corev1.EventTypeNormal, model.SuccessfulNodepoolCreateEventReason, "Created NodePool %q", npi.Name())
		return nil
	}

	var np *karpenterv1.NodePool

	// Create replica of original NodePool if Target exists; otherwise use NodePoolInternal to create a NodePool
	if targetNp != nil {
		log.Debugf("Building replica of NodePool: %s", npi.TargetName())
		np = model.BuildReplicaNodePool(targetNp, npi)
	} else {
		nodeClassRef, err := c.discoverNodeClass(ctx)
		if err != nil {
			return fmt.Errorf("unable to discover node class for NodePool: %s, err: %v", npi.Name(), err)
		}
		np = model.ConvertToKarpenterNodePool(npi, nodeClassRef)
	}

	npUnstr, err := convertNodePoolToUnstructured(np)
	if err != nil {
		return fmt.Errorf("unable to convert NodePool to unstructured: %s, err: %v", npi.Name(), err)
	}

	_, err = c.Client.Resource(nodePoolGVR).Create(ctx, npUnstr, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("unable to create NodePool: %s, err: %v", npi.Name(), err)
	}

	c.eventRecorder.Eventf(np, corev1.EventTypeNormal, model.SuccessfulNodepoolCreateEventReason, "Created NodePool with instances %q", npi.RecommendedInstanceTypes())

	return nil
}

func (c *Controller) updateNodePool(ctx context.Context, targetNp, datadogNp *karpenterv1.NodePool, npi model.NodePoolInternal) error {
	// New path: update from manifest-provided NodePool when available
	if desired := npi.KarpenterNodePool().DeepCopy(); desired != nil {
		if desired.Labels == nil {
			desired.Labels = make(map[string]string)
		}
		// Preserve DatadogCreatedLabelKey from the live object so the controller
		// continues to treat this NodePool as fully managed after updates.
		if _, ok := datadogNp.Labels[model.DatadogCreatedLabelKey]; ok {
			desired.Labels[model.DatadogCreatedLabelKey] = "true"
		}

		// Use the NodeClass in the live NodePool if the manifest omits it
		if desired.Spec.Template.Spec.NodeClassRef == nil && datadogNp.Spec.Template.Spec.NodeClassRef != nil {
			desired.Spec.Template.Spec.NodeClassRef = datadogNp.Spec.Template.Spec.NodeClassRef.DeepCopy()
		}
		var err error
		desired, err = c.checkValidNodeClass(ctx, desired)
		if err != nil {
			return fmt.Errorf("unable to update NodePool with node class: %s, err: %v", npi.Name(), err)
		}

		// Ensure Datadog autoscaling node label is always present
		if desired.Spec.Template.ObjectMeta.Labels == nil {
			desired.Spec.Template.ObjectMeta.Labels = make(map[string]string)
		}
		desired.Spec.Template.ObjectMeta.Labels[kubernetes.AutoscalingLabelKey] = "true"

		// Use merge-patch for spec comparison so fields added to NodePool by default do not trigger unnecessary updates
		liveSpecJSON, err := json.Marshal(datadogNp.Spec)
		if err != nil {
			return fmt.Errorf("unable to marshal live NodePool spec: %s, err: %v", npi.Name(), err)
		}
		desiredSpecJSON, err := json.Marshal(desired.Spec)
		if err != nil {
			return fmt.Errorf("unable to marshal desired NodePool spec: %s, err: %v", npi.Name(), err)
		}
		mergedSpecJSON, err := jsonpatch.MergePatch(liveSpecJSON, desiredSpecJSON)
		if err != nil {
			return fmt.Errorf("unable to compute spec merge patch for NodePool: %s, err: %v", npi.Name(), err)
		}

		// Ensure DatadogReplicaAnnotationKey is always present when a target exists
		if desired.Annotations == nil {
			desired.Annotations = make(map[string]string)
		}
		if npi.TargetName() != "" {
			desired.Annotations[model.DatadogReplicaAnnotationKey] = npi.TargetName()
		}

		// Ignore any annotations managed by Karpenter
		annotationsMatch := true
		for k, v := range desired.Annotations {
			if datadogNp.Annotations[k] != v {
				annotationsMatch = false
				break
			}
		}

		if bytes.Equal(liveSpecJSON, mergedSpecJSON) &&
			maps.Equal(datadogNp.Labels, desired.Labels) &&
			annotationsMatch {
			log.Debugf("NodePool: %s has not changed, no action will be applied.", npi.Name())
			return nil
		}

		log.Infof("Updating NodePool: %s", npi.Name())
		desired.ResourceVersion = datadogNp.ResourceVersion
		updatedUnstr, err := convertNodePoolToUnstructured(desired)
		if err != nil {
			c.eventRecorder.Eventf(datadogNp, corev1.EventTypeWarning, model.FailedNodepoolUpdateEventReason, "Failed to convert NodePool: %v", err)
			return fmt.Errorf("error converting NodePool to unstructured: %s, err: %v", npi.Name(), err)
		}
		_, err = c.Client.Resource(nodePoolGVR).Update(ctx, updatedUnstr, metav1.UpdateOptions{})
		if err != nil {
			c.eventRecorder.Eventf(datadogNp, corev1.EventTypeWarning, model.FailedNodepoolUpdateEventReason, "Failed to update NodePool: %v", err)
			return fmt.Errorf("unable to update NodePool: %s, err: %v", npi.Name(), err)
		}
		c.eventRecorder.Eventf(datadogNp, corev1.EventTypeNormal, model.SuccessfulNodepoolUpdateEventReason, "Updated NodePool %q", npi.Name())
		return nil
	}

	// Apply updates from NodePoolInternal to the NodePool object
	desiredNp := model.UpdateNodePoolObject(targetNp, datadogNp, npi)
	// Compare entire Spec
	if equality.Semantic.DeepEqual(datadogNp.Spec, desiredNp.Spec) && maps.Equal(datadogNp.GetLabels(), desiredNp.GetLabels()) {
		log.Debugf("NodePool: %s spec and labels have not changed, no action will be applied.", npi.Name())
		return nil
	}

	log.Infof("Updating NodePool: %s", npi.Name())

	// Convert to unstructured
	updatedUnstr, err := convertNodePoolToUnstructured(&desiredNp)
	if err != nil {
		c.eventRecorder.Eventf(datadogNp, corev1.EventTypeWarning, model.FailedNodepoolUpdateEventReason, "Failed to convert NodePool: %v", err)
		return fmt.Errorf("error converting NodePool to unstructured: %s, err: %v", npi.Name(), err)
	}

	// Update the NodePool
	_, err = c.Client.Resource(nodePoolGVR).Update(ctx, updatedUnstr, metav1.UpdateOptions{})
	if err != nil {
		c.eventRecorder.Eventf(datadogNp, corev1.EventTypeWarning, model.FailedNodepoolUpdateEventReason, "Failed to update NodePool: %v", err)
		return fmt.Errorf("unable to update NodePool: %s, err: %v", npi.Name(), err)
	}

	c.eventRecorder.Eventf(datadogNp, corev1.EventTypeNormal, model.SuccessfulNodepoolUpdateEventReason, "Updated NodePool with instances %q", npi.RecommendedInstanceTypes())
	return nil
}

func (c *Controller) deleteNodePool(ctx context.Context, name string, knp *karpenterv1.NodePool) error {
	log.Infof("Deleting NodePool: %s", name)

	err := c.Client.Resource(nodePoolGVR).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		c.eventRecorder.Eventf(knp, corev1.EventTypeWarning, model.FailedNodepoolDeleteEventReason, "Failed to delete NodePool: %v", err)
		return fmt.Errorf("Unable to delete NodePool: %s, err: %v", name, err)
	}

	c.eventRecorder.Eventf(knp, corev1.EventTypeNormal, model.SuccessfulNodepoolDeleteEventReason, "Deleted NodePool: %s", name)
	return nil
}

func (c *Controller) checkValidNodeClass(ctx context.Context, knp *karpenterv1.NodePool) (*karpenterv1.NodePool, error) {
	nc := knp.Spec.Template.Spec.NodeClassRef
	if nc != nil {
		gvr, ok := nodeClassGVRByGroup[nc.Group]
		if !ok {
			return nil, fmt.Errorf("unknown NodeClassRef group %q", nc.Group)
		}
		_, err := c.Client.Resource(gvr).Get(ctx, nc.Name, metav1.GetOptions{})
		if err == nil { // nodeClassRef is valid, keep it
			return knp, nil
		}
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("unable to validate NodeClassRef %q: %w", nc.Name, err)
		}
		log.Debugf("NodeClass %s not found, falling back to an existing NodeClass", nc.Name)
	}

	// Get NodeClass. If there's none or more than one, then we should not create the NodePool
	nodeClassRef, err := c.discoverNodeClass(ctx)
	if err != nil {
		return nil, err
	}
	knp.Spec.Template.Spec.NodeClassRef = nodeClassRef
	return knp, nil
}

// discoverNodeClass attempts to find a single node class from supported providers.
// It tries manual Karpenter (EC2NodeClass) first, then falls back to EKS Auto Mode (NodeClass).
// Returns the NodeClassReference for the discovered node class, or an error if none or too many are found.
func (c *Controller) discoverNodeClass(ctx context.Context) (*karpenterv1.NodeClassReference, error) {
	for _, provider := range []struct {
		gvr  schema.GroupVersionResource
		kind string
	}{
		{gvr: ec2NodeClassGVR, kind: "EC2NodeClass"},
		{gvr: eksNodeClassGVR, kind: "NodeClass"},
	} {
		ncList, err := c.Client.Resource(provider.gvr).List(ctx, metav1.ListOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Debugf("NodeClass CRD %s/%s not found, trying next provider", provider.gvr.Group, provider.kind)
				continue
			}
			return nil, fmt.Errorf("unable to list %s/%s NodeClasses: %w", provider.gvr.Group, provider.kind, err)
		}

		if len(ncList.Items) == 0 {
			continue
		}

		if len(ncList.Items) > 1 {
			return nil, fmt.Errorf("too many %s NodeClasses found (%d), NodePool cannot be created", provider.gvr.Group, len(ncList.Items))
		}

		return &karpenterv1.NodeClassReference{
			Kind:  provider.kind,
			Name:  ncList.Items[0].GetName(),
			Group: provider.gvr.Group,
		}, nil
	}

	return nil, errors.New("no NodeClasses found from any supported provider, NodePool cannot be created")
}

func isCreatedByDatadog(labels map[string]string) bool {
	if _, ok := labels[model.DatadogCreatedLabelKey]; ok {
		return true
	}
	return false
}

// Helper function to convert a typed Karpenter NodePool object to unstructured. Handles custom Go types gracefully
func convertNodePoolToUnstructured(np interface{}) (*unstructured.Unstructured, error) {
	// Marshal the structured object to JSON bytes.
	jsonBytes, err := json.Marshal(np)
	if err != nil {
		return nil, err
	}

	// Unmarshal the JSON bytes into a map[string]interface{}.
	var unstructuredMap map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &unstructuredMap); err != nil {
		return nil, err
	}

	// Wrap the map in unstructured.Unstructured.
	unstructuredObj := &unstructured.Unstructured{
		Object: unstructuredMap,
	}

	return unstructuredObj, nil
}

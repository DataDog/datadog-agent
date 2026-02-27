// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type store = autoscaling.Store[model.NodePoolInternal]

var (
	nodePoolGVR = schema.GroupVersionResource{Group: "karpenter.sh", Version: "v1", Resource: "nodepools"}
	// Only support EC2 for now
	nodeClassGVR = schema.GroupVersionResource{Group: "karpenter.k8s.aws", Version: "v1", Resource: "ec2nodeclasses"}

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
	npi, foundInStore := c.store.LockRead(name, true)
	defer c.store.Unlock(name)

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

	var np *karpenterv1.NodePool

	// Create replica of original NodePool if Target exists; otherwise use NodePoolInternal to create a NodePool
	if targetNp != nil {
		log.Debugf("Building replica of NodePool: %s", npi.TargetName())
		np = model.BuildReplicaNodePool(targetNp, npi)
	} else {
		// Get NodeClass. If there's none or more than one, then we should not create the NodePool
		ncList, err := c.Client.Resource(nodeClassGVR).List(ctx, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("unable to list NodeClasses: %w", err)
		}

		if len(ncList.Items) == 0 {
			return errors.New("no NodeClasses found, NodePool cannot be created")
		}

		if len(ncList.Items) > 1 {
			return fmt.Errorf("too many NodeClasses found (%v), NodePool cannot be created", len(ncList.Items))
		}

		u := ncList.Items[0]
		np = model.ConvertToKarpenterNodePool(npi, u.GetName())
	}

	npUnstr, err := convertNodePoolToUnstructured(np)
	if err != nil {
		return err
	}

	_, err = c.Client.Resource(nodePoolGVR).Create(ctx, npUnstr, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("unable to create NodePool: %s, err: %v", npi.Name(), err)
	}

	c.eventRecorder.Eventf(np, corev1.EventTypeNormal, model.SuccessfulNodepoolCreateEventReason, "Created NodePool with instances %q", npi.RecommendedInstanceTypes())

	return nil
}

func (c *Controller) updateNodePool(ctx context.Context, targetNp, datadogNp *karpenterv1.NodePool, npi model.NodePoolInternal) error {

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

func isCreatedByDatadog(labels map[string]string) bool {
	if _, ok := labels[model.DatadogCreatedLabelKey]; ok {
		return true
	}
	return false
}

// Helper function to convert a typed Karpenter NodePool object to unstructured. Handles custom Go types gracefully
func convertNodePoolToUnstructured(np interface{}) (*unstructured.Unstructured, error) {
	// Marshal the structured object to JSON bytes.
	bytes, err := json.Marshal(np)
	if err != nil {
		return nil, err
	}

	// Unmarshal the JSON bytes into a map[string]interface{}.
	var unstructuredMap map[string]interface{}
	if err := json.Unmarshal(bytes, &unstructuredMap); err != nil {
		return nil, err
	}

	// Wrap the map in unstructured.Unstructured.
	unstructuredObj := &unstructured.Unstructured{
		Object: unstructuredMap,
	}

	return unstructuredObj, nil
}

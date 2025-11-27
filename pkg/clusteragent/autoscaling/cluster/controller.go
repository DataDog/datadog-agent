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
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

type store = autoscaling.Store[model.NodePoolInternal]

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
func (c *Controller) Process(ctx context.Context, _, ns, name string) autoscaling.ProcessResult {
	if !c.IsLeader() || !*c.storeUpdated {
		return autoscaling.ProcessResult{}
	}

	// Try to get Datadog-managed NodePool from cluster
	nodePool := &karpenterv1.NodePool{}
	npUnstr, err := c.Lister.Get(name)
	if err == nil {
		err = autoscaling.FromUnstructured(npUnstr, nodePool)
	}

	switch {
	case apierrors.IsNotFound(err):
		// Ignore not found error as it will be created later
		nodePool = nil
	case err != nil:
		log.Errorf("Unable to retrieve NodePool: %w", err)
		return autoscaling.Requeue
	case npUnstr == nil:
		log.Errorf("Could not parse empty NodePool from local cache")
		return autoscaling.Requeue
	}

	return c.syncNodePool(ctx, name, nodePool)
}

func (c *Controller) syncNodePool(ctx context.Context, name string, nodePool *karpenterv1.NodePool) autoscaling.ProcessResult {
	// TODO create duplicate NodePools with greater weight, rather than updating user NodePools
	npi, foundInStore := c.store.LockRead(name, true)
	defer c.store.Unlock(name)

	if foundInStore {
		if nodePool == nil {
			// Present in store but not found in cluster; create it
			if err := c.createNodePool(ctx, npi); err != nil {
				log.Errorf("Error creating NodePool: %v", err)
				return autoscaling.Requeue
			}
		} else {
			// Present in store and found in cluster; update it
			// TODO check if hash of spec from remote config matches current object before updating
			if err := c.patchNodePool(ctx, nodePool, npi); err != nil {
				log.Errorf("Error updating NodePool: %v", err)
				return autoscaling.Requeue
			}
		}
	} else {
		if nodePool != nil && isCreatedByDatadog(nodePool.GetLabels()) {
			// Not present in store, and the cluster NodePool is fully managed, then delete the NodePool
			if err := c.deleteNodePool(ctx, name); err != nil {
				log.Errorf("Error deleting NodePool: %v", err)
				return autoscaling.Requeue
			}
		} else {
			// Not present in store and the cluster NodePool is not fully managed, do nothing
			log.Debugf("NodePool %s not found in store and is not fully managed, nothing to do", name)
		}
	}

	return autoscaling.ProcessResult{}
}

func (c *Controller) createNodePool(ctx context.Context, npi model.NodePoolInternal) error {
	log.Infof("Creating NodePool: %s", npi.Name())

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
	npObj := model.ConvertToKarpenterNodePool(npi, u.GetName())

	npUnstr, err := autoscaling.ToUnstructured(npObj)
	if err != nil {
		return err
	}

	_, err = c.Client.Resource(nodePoolGVR).Create(ctx, npUnstr, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("unable to create NodePool: %s, err: %v", npi.Name(), err)
	}

	return nil
}

func (c *Controller) patchNodePool(ctx context.Context, knp *karpenterv1.NodePool, npi model.NodePoolInternal) error {
	log.Infof("Patching NodePool: %s", npi.Name())

	patchData := model.BuildNodePoolPatch(knp, npi)
	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		return fmt.Errorf("error marshaling patch data: %s, err: %v", npi.Name(), err)
	}

	// TODO: If NodePool is not considered a custom resource in the future, use StrategicMergePatchType and simplify patch object
	_, err = c.Client.Resource(nodePoolGVR).Patch(ctx, npi.Name(), types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("unable to update NodePool: %s, err: %v", npi.Name(), err)
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

func isCreatedByDatadog(labels map[string]string) bool {
	if _, ok := labels[model.DatadogCreatedLabelKey]; ok {
		return true
	}
	return false
}

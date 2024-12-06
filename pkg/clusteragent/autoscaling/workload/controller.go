// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	scaleclient "k8s.io/client-go/scale"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

const (
	// TODO: evaluate the retry values vs backoff time of the workqueue
	maxRetry int = 5

	controllerID = "dpa-c"
)

var (
	podAutoscalerGVR  = datadoghq.GroupVersion.WithResource("datadogpodautoscalers")
	podAutoscalerMeta = metav1.TypeMeta{
		Kind:       "DatadogPodAutoscaler",
		APIVersion: "datadoghq.com/v1alpha1",
	}
)

type store = autoscaling.Store[model.PodAutoscalerInternal]

// Controller for DatadogPodAutoscaler objects
type Controller struct {
	*autoscaling.Controller

	clusterID string
	clock     clock.Clock

	eventRecorder record.EventRecorder
	store         *store

	limitHeap *autoscaling.HashHeap

	podWatcher           podWatcher
	horizontalController *horizontalController
	verticalController   *verticalController

	localSender sender.Sender
}

// newController returns a new workload autoscaling controller
func newController(
	clusterID string,
	eventRecorder record.EventRecorder,
	restMapper apimeta.RESTMapper,
	scaleClient scaleclient.ScalesGetter,
	dynamicClient dynamic.Interface,
	dynamicInformer dynamicinformer.DynamicSharedInformerFactory,
	isLeader func() bool,
	store *store,
	podWatcher podWatcher,
	localSender sender.Sender,
	limitHeap *autoscaling.HashHeap,
) (*Controller, error) {
	c := &Controller{
		clusterID:     clusterID,
		clock:         clock.RealClock{},
		eventRecorder: eventRecorder,
		localSender:   localSender,
	}

	autoscalingWorkqueue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedItemBasedRateLimiter[string](),
		workqueue.TypedRateLimitingQueueConfig[string]{
			Name:            subsystem,
			MetricsProvider: autoscalingQueueMetricsProvider,
		},
	)

	baseController, err := autoscaling.NewController(controllerID, c, dynamicClient, dynamicInformer, podAutoscalerGVR, isLeader, store, autoscalingWorkqueue)
	if err != nil {
		return nil, err
	}

	c.Controller = baseController
	c.limitHeap = limitHeap
	store.RegisterObserver(autoscaling.Observer{
		SetFunc:    c.limitHeap.InsertIntoHeap,
		DeleteFunc: c.limitHeap.DeleteFromHeap,
	})
	c.store = store
	c.podWatcher = podWatcher

	// TODO: Ensure that controllers do not take action before the podwatcher is synced
	c.horizontalController = newHorizontalReconciler(c.clock, eventRecorder, restMapper, scaleClient)
	c.verticalController = newVerticalController(c.clock, eventRecorder, dynamicClient, c.podWatcher)

	return c, nil
}

// PreStart is called before the controller starts
func (c *Controller) PreStart(ctx context.Context) {
	startLocalTelemetry(ctx, c.localSender, []string{"kube_cluster_id:" + c.clusterID})
}

// Process implements the Processor interface (so required to be public)
func (c *Controller) Process(ctx context.Context, key, ns, name string) autoscaling.ProcessResult {
	res, err := c.processPodAutoscaler(ctx, key, ns, name)
	if err != nil {
		numRequeues := c.Workqueue.NumRequeues(key)
		log.Errorf("Impossible to synchronize DatadogPodAutoscaler (attempt #%d): %s, err: %v", numRequeues, key, err)

		if numRequeues >= maxRetry {
			log.Infof("Max retries reached for DatadogPodAutoscaler: %s, removing from queue", key)
			res = autoscaling.NoRequeue
		}
	}

	log.Debugf("Processed DatadogPodAutoscaler: %s, result: %+v", key, res)
	return res
}

func (c *Controller) processPodAutoscaler(ctx context.Context, key, ns, name string) (autoscaling.ProcessResult, error) {
	podAutoscaler := &datadoghq.DatadogPodAutoscaler{}
	podAutoscalerCachedObj, err := c.Lister.ByNamespace(ns).Get(name)
	if err == nil {
		err = autoscaling.FromUnstructured(podAutoscalerCachedObj, podAutoscaler)
	}

	switch {
	case errors.IsNotFound(err):
		// We ignore not found here as we may need to create a DatadogPodAutoscaler later
		podAutoscaler = nil
	case err != nil:
		return autoscaling.Requeue, fmt.Errorf("Unable to retrieve DatadogPodAutoscaler: %w", err)
	case podAutoscalerCachedObj == nil:
		return autoscaling.Requeue, fmt.Errorf("Could not parse empty DatadogPodAutoscaler from local cache")
	}

	// No error path, check what to do with this event
	if c.IsLeader() {
		return c.syncPodAutoscaler(ctx, key, ns, name, podAutoscaler)
	}

	// In follower mode, we simply sync updates from Kubernetes
	// If the object is present in Kubernetes, we will update our local version
	// Otherwise, we clear it from our local store
	if podAutoscaler != nil {
		c.store.Set(key, model.NewPodAutoscalerInternal(podAutoscaler), c.ID)
	} else {
		c.store.Delete(key, c.ID)
	}

	return autoscaling.NoRequeue, nil
}

// Synchronize DatadogPodAutoscaler state between internal store and Kubernetes objects
// Make sure any `return` has the proper store Unlock
// podAutoscaler is read-only, any changes require a DeepCopy
func (c *Controller) syncPodAutoscaler(ctx context.Context, key, ns, name string, podAutoscaler *datadoghq.DatadogPodAutoscaler) (autoscaling.ProcessResult, error) {
	podAutoscalerInternal, podAutoscalerInternalFound := c.store.LockRead(key, true)

	// Object is missing from our store
	if !podAutoscalerInternalFound {
		if podAutoscaler != nil {
			// If we don't have an instance locally, we create it. Deletion is handled through setting the `Deleted` flag
			log.Debugf("Creating internal PodAutoscaler: %s from Kubernetes object", key)
			c.store.UnlockSet(key, model.NewPodAutoscalerInternal(podAutoscaler), c.ID)
		} else {
			// If podAutoscaler == nil, both objects are nil, nothing to do
			log.Debugf("Reconciling object: %s but object is not present in Kubernetes nor in internal store, nothing to do", key)
			c.store.Unlock(key)
		}

		return autoscaling.NoRequeue, nil
	}

	if podAutoscaler == nil {
		// Object is not present in Kubernetes
		// If flagged for deletion, we just need to clear up our store (deletion complete)
		// Also if object was not owned by remote config, we also need to delete it (deleted by user)
		if podAutoscalerInternal.Deleted() || podAutoscalerInternal.Spec().Owner != datadoghq.DatadogPodAutoscalerRemoteOwner {
			log.Infof("Object %s not present in Kubernetes and flagged for deletion (remote) or owner == local, clearing internal store", key)
			c.store.UnlockDelete(key, c.ID)
			return autoscaling.NoRequeue, nil
		}

		// Object is not flagged for deletion and owned by remote config, we need to create it in Kubernetes
		log.Infof("Object %s has remote owner and not present in Kubernetes, creating it", key)
		err := c.createPodAutoscaler(ctx, podAutoscalerInternal)

		c.store.Unlock(key)
		return autoscaling.Requeue, err
	}

	// Object is present in both our store and Kubernetes, we need to sync depending on ownership.
	// Implement info sync based on ownership.
	if podAutoscaler.Spec.Owner == datadoghq.DatadogPodAutoscalerRemoteOwner {
		// First implement deletion logic, as if it's a deletion, we don't need to update the object.
		// Deletion can only happen if the object is owned by remote config.
		if podAutoscalerInternal.Deleted() {
			log.Infof("Remote owned PodAutoscaler with Deleted flag, deleting object: %s", key)
			err := c.deletePodAutoscaler(ns, name)
			// In case of not found, it means the object is gone but informer cache is not updated yet, we can safely delete it from our store
			if err != nil && errors.IsNotFound(err) {
				log.Debugf("Object %s not found in Kubernetes during deletion, clearing internal store", key)
				c.store.UnlockDelete(key, c.ID)
				return autoscaling.NoRequeue, nil
			}

			// In all other cases, we requeue and wait for the object to be deleted from store with next reconcile
			c.store.Unlock(key)
			return autoscaling.Requeue, err
		}

		// If the object is owned by remote config and newer, we need to update the spec in Kubernetes
		// If Kubernetes is newer, we wait for RC to update the object in our internal store.
		if podAutoscalerInternal.Spec().RemoteVersion != nil &&
			podAutoscaler.Spec.RemoteVersion != nil &&
			*podAutoscalerInternal.Spec().RemoteVersion > *podAutoscaler.Spec.RemoteVersion {
			err := c.updatePodAutoscalerSpec(ctx, podAutoscalerInternal, podAutoscaler)

			// When doing an external update, we stop and requeue the object to not have multiple changes at once.
			c.store.Unlock(key)
			return autoscaling.Requeue, err
		}

		// If Generation != podAutoscaler.Generation, we should compute `.Spec` hash
		// and compare it with the one in the PodAutoscaler. If they differ, we should update the PodAutoscaler
		// otherwise store the Generation
		if podAutoscalerInternal.Generation() != podAutoscaler.Generation {
			localHash, err := autoscaling.ObjectHash(podAutoscalerInternal.Spec())
			if err != nil {
				c.store.Unlock(key)
				return autoscaling.Requeue, fmt.Errorf("Failed to compute Spec hash for PodAutoscaler: %s/%s, err: %v", ns, name, err)
			}

			remoteHash, err := autoscaling.ObjectHash(&podAutoscaler.Spec)
			if err != nil {
				c.store.Unlock(key)
				return autoscaling.Requeue, fmt.Errorf("Failed to compute Spec hash for PodAutoscaler: %s/%s, err: %v", ns, name, err)
			}

			if localHash != remoteHash {
				err := c.updatePodAutoscalerSpec(ctx, podAutoscalerInternal, podAutoscaler)

				// When doing an external update, we stop and requeue the object to not have multiple changes at once.
				c.store.Unlock(key)
				return autoscaling.Requeue, err
			}

			podAutoscalerInternal.SetGeneration(podAutoscaler.Generation)
			if podAutoscalerInternal.CreationTimestamp().IsZero() {
				podAutoscalerInternal.UpdateCreationTimestamp(podAutoscaler.CreationTimestamp.Time)
			}
		}
	}

	// Implement sync logic for local ownership, source of truth is Kubernetes
	if podAutoscalerInternal.Spec().Owner == datadoghq.DatadogPodAutoscalerLocalOwner {
		if podAutoscalerInternal.Generation() != podAutoscaler.Generation {
			podAutoscalerInternal.UpdateFromPodAutoscaler(podAutoscaler)
		}
	}

	// Reaching this point, we had an error in processing, clearing up global error
	podAutoscalerInternal.SetError(nil)

	// Validate autoscaler requirements
	validationErr := c.validateAutoscaler(podAutoscalerInternal)
	if validationErr != nil {
		podAutoscalerInternal.SetError(validationErr)
		return autoscaling.NoRequeue, c.updateAutoscalerStatusAndUnlock(ctx, key, ns, name, validationErr, podAutoscalerInternal, podAutoscaler)
	}

	// Get autoscaler target
	targetGVK, targetErr := podAutoscalerInternal.TargetGVK()
	if targetErr != nil {
		podAutoscalerInternal.SetError(targetErr)
		return autoscaling.NoRequeue, c.updateAutoscalerStatusAndUnlock(ctx, key, ns, name, targetErr, podAutoscalerInternal, podAutoscaler)
	}
	target := NamespacedPodOwner{
		Namespace: podAutoscalerInternal.Namespace(),
		Name:      podAutoscalerInternal.Spec().TargetRef.Name,
		Kind:      targetGVK.Kind,
	}

	// Now that everything is synced, we can perform the actual processing
	result, scalingErr := c.handleScaling(ctx, podAutoscaler, &podAutoscalerInternal, targetGVK, target)

	// Update current replicas
	pods := c.podWatcher.GetPodsForOwner(target)
	currentReplicas := len(pods)
	podAutoscalerInternal.SetCurrentReplicas(int32(currentReplicas))

	// Update status based on latest state
	return result, c.updateAutoscalerStatusAndUnlock(ctx, key, ns, name, scalingErr, podAutoscalerInternal, podAutoscaler)
}

func (c *Controller) handleScaling(ctx context.Context, podAutoscaler *datadoghq.DatadogPodAutoscaler, podAutoscalerInternal *model.PodAutoscalerInternal, targetGVK schema.GroupVersionKind, target NamespacedPodOwner) (autoscaling.ProcessResult, error) {
	// TODO: While horizontal scaling is in progress we should not start vertical scaling
	// While vertical scaling is in progress we should only allow horizontal upscale
	horizontalRes, err := c.horizontalController.sync(ctx, podAutoscaler, podAutoscalerInternal)
	if err != nil {
		return horizontalRes, err
	}

	verticalRes, err := c.verticalController.sync(ctx, podAutoscaler, podAutoscalerInternal, targetGVK, target)
	if err != nil {
		return verticalRes, err
	}

	return horizontalRes.Merge(verticalRes), nil
}

func (c *Controller) createPodAutoscaler(ctx context.Context, podAutoscalerInternal model.PodAutoscalerInternal) error {
	log.Infof("Creating PodAutoscaler Spec: %s/%s", podAutoscalerInternal.Namespace(), podAutoscalerInternal.Name())
	autoscalerObj := &datadoghq.DatadogPodAutoscaler{
		TypeMeta: podAutoscalerMeta,
		ObjectMeta: metav1.ObjectMeta{
			Namespace: podAutoscalerInternal.Namespace(),
			Name:      podAutoscalerInternal.Name(),
		},
		Spec:   *podAutoscalerInternal.Spec().DeepCopy(),
		Status: podAutoscalerInternal.BuildStatus(metav1.NewTime(c.clock.Now()), nil),
	}
	trackPodAutoscalerStatus(autoscalerObj)

	obj, err := autoscaling.ToUnstructured(autoscalerObj)
	if err != nil {
		return err
	}

	_, err = c.Client.Resource(podAutoscalerGVR).Namespace(podAutoscalerInternal.Namespace()).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("Unable to create PodAutoscaler: %s/%s, err: %v", podAutoscalerInternal.Namespace(), podAutoscalerInternal.Name(), err)
	}

	return nil
}

func (c *Controller) updatePodAutoscalerSpec(ctx context.Context, podAutoscalerInternal model.PodAutoscalerInternal, podAutoscaler *datadoghq.DatadogPodAutoscaler) error {
	log.Infof("Updating PodAutoscaler Spec: %s/%s", podAutoscalerInternal.Namespace(), podAutoscalerInternal.Name())
	autoscalerObj := &datadoghq.DatadogPodAutoscaler{
		TypeMeta:   podAutoscalerMeta,
		ObjectMeta: podAutoscaler.ObjectMeta,
		Spec:       *podAutoscalerInternal.Spec().DeepCopy(),
	}

	obj, err := autoscaling.ToUnstructured(autoscalerObj)
	if err != nil {
		return err
	}

	_, err = c.Client.Resource(podAutoscalerGVR).Namespace(podAutoscalerInternal.Namespace()).Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("Unable to update PodAutoscaler Spec: %s/%s, err: %w", podAutoscalerInternal.Namespace(), podAutoscalerInternal.Name(), err)
	}

	return nil
}

func (c *Controller) updatePodAutoscalerStatus(ctx context.Context, podAutoscalerInternal model.PodAutoscalerInternal, podAutoscaler *datadoghq.DatadogPodAutoscaler) error {
	newStatus := podAutoscalerInternal.BuildStatus(metav1.NewTime(c.clock.Now()), &podAutoscaler.Status)

	if autoscaling.Semantic.DeepEqual(podAutoscaler.Status, newStatus) {
		return nil
	}

	log.Debugf("Updating PodAutoscaler Status: %s/%s", podAutoscalerInternal.Namespace(), podAutoscalerInternal.Name())
	autoscalerObj := &datadoghq.DatadogPodAutoscaler{
		TypeMeta:   podAutoscalerMeta,
		ObjectMeta: podAutoscaler.ObjectMeta,
		Status:     newStatus,
	}
	trackPodAutoscalerStatus(autoscalerObj)

	obj, err := autoscaling.ToUnstructured(autoscalerObj)
	if err != nil {
		return err
	}

	_, err = c.Client.Resource(podAutoscalerGVR).Namespace(podAutoscalerInternal.Namespace()).UpdateStatus(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("Unable to update PodAutoscaler Status: %s/%s, err: %w", podAutoscalerInternal.Namespace(), podAutoscalerInternal.Name(), err)
	}

	return nil
}

func (c *Controller) deletePodAutoscaler(ns, name string) error {
	log.Infof("Deleting PodAutoscaler: %s/%s", ns, name)
	err := c.Client.Resource(podAutoscalerGVR).Namespace(ns).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("Unable to delete PodAutoscaler: %s/%s, err: %v", ns, name, err)
	}
	return nil
}

func (c *Controller) validateAutoscaler(podAutoscalerInternal model.PodAutoscalerInternal) error {
	// Check that we are within the limit of 100 DatadogPodAutoscalers
	key := podAutoscalerInternal.ID()
	if !c.limitHeap.Exists(key) {
		return fmt.Errorf("Autoscaler disabled as maximum number per cluster reached (%d)", maxDatadogPodAutoscalerObjects)
	}

	// Check that targetRef is not set to the cluster agent
	clusterAgentPodName, err := common.GetSelfPodName()
	// If we cannot get cluster agent pod name, just skip the validation logic
	if err != nil {
		return nil
	}

	var resourceName string
	switch owner := podAutoscalerInternal.Spec().TargetRef.Kind; owner {
	case "Deployment":
		resourceName = kubernetes.ParseDeploymentForPodName(clusterAgentPodName)
	case "ReplicaSet":
		resourceName = kubernetes.ParseReplicaSetForPodName(clusterAgentPodName)
	}

	clusterAgentNs := common.GetMyNamespace()

	if podAutoscalerInternal.Namespace() == clusterAgentNs && podAutoscalerInternal.Spec().TargetRef.Name == resourceName {
		return fmt.Errorf("Autoscaling target cannot be set to the cluster agent")
	}
	return nil
}

func (c *Controller) updateAutoscalerStatusAndUnlock(ctx context.Context, key, ns, name string, err error, podAutoscalerInternal model.PodAutoscalerInternal, podAutoscaler *datadoghq.DatadogPodAutoscaler) error {
	// Update status based on latest state
	statusErr := c.updatePodAutoscalerStatus(ctx, podAutoscalerInternal, podAutoscaler)
	if statusErr != nil {
		log.Errorf("Failed to update status for PodAutoscaler: %s/%s, err: %v", ns, name, statusErr)

		// We want to return the status error if none to count in the requeue retries.
		if err == nil {
			err = statusErr
		}
	}

	c.store.UnlockSet(key, podAutoscalerInternal, c.ID)
	return err
}

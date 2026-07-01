// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package profile

import (
	"context"
	"errors"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	controllerID autoscaling.SenderID = "dpap-c"

	maxRetry = 1
)

var (
	podAutoscalerClusterProfileGVR  = datadoghq.GroupVersion.WithResource("datadogpodautoscalerclusterprofiles")
	podAutoscalerClusterProfileMeta = metav1.TypeMeta{
		Kind:       "DatadogPodAutoscalerClusterProfile",
		APIVersion: "datadoghq.com/v1alpha2",
	}
)

type (
	profileStore = autoscaling.Store[model.PodAutoscalerProfileInternal]
)

// Controller watches DatadogPodAutoscalerClusterProfile CRDs, validates them, and
// manages the profile store. DPA lifecycle is handled by the AutoscalerSyncer.
type Controller struct {
	*autoscaling.Controller

	clock clock.Clock
	store *profileStore
}

// NewController creates a new Profile Controller using the autoscaling framework.
// The readyC channel is closed once the initial profile sync is complete;
// it is created by the caller so other components can wait on it without
// holding a reference to the Controller.
func NewController(
	clock clock.Clock,
	client dynamic.Interface,
	informerFactory dynamicinformer.DynamicSharedInformerFactory,
	isLeader func() bool,
	store *profileStore,
) (*Controller, error) {
	c := &Controller{
		clock: clock,
		store: store,
	}

	autoscalingWorkqueue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedItemBasedRateLimiter[string](),
		workqueue.TypedRateLimitingQueueConfig[string]{
			Name: string(controllerID),
		},
	)

	baseController, err := autoscaling.NewController(
		controllerID, c, client, informerFactory,
		podAutoscalerClusterProfileGVR, isLeader, store, autoscalingWorkqueue,
	)
	if err != nil {
		return nil, err
	}

	c.Controller = baseController
	return c, nil
}

func (c *Controller) Process(ctx context.Context, _, _, name string) autoscaling.ProcessResult {
	// key and name are the same for profile controller (no namespace)
	res, err := c.processProfile(ctx, name)
	if err != nil {
		numRequeues := c.Workqueue.NumRequeues(name)
		log.Errorf("Impossible to synchronize DatadogPodAutoscalerClusterProfile (attempt #%d): %s, err: %v", numRequeues, name, err)

		if numRequeues >= maxRetry {
			log.Infof("Max retries reached for DatadogPodAutoscalerClusterProfile: %s, removing from queue", name)
			res = autoscaling.NoRequeue
		}
	}

	log.Debugf("Processed DatadogPodAutoscalerClusterProfile: %s, result: %+v", name, res)
	return res
}

// Process implements the autoscaling.Processor interface.
func (c *Controller) processProfile(ctx context.Context, name string) (autoscaling.ProcessResult, error) {
	profile := &datadoghq.DatadogPodAutoscalerClusterProfile{}
	profileCachedObj, err := c.Lister.Get(name)
	if err == nil {
		err = autoscaling.FromUnstructured(profileCachedObj, profile)
	}

	switch {
	case k8serrors.IsNotFound(err):
		profile = nil
	case err != nil:
		return autoscaling.Requeue, fmt.Errorf("Unable to retrieve DatadogPodAutoscalerClusterProfile: %w", err)
	case profileCachedObj == nil:
		return autoscaling.Requeue, errors.New("Could not parse empty DatadogPodAutoscalerClusterProfile from local cache")
	}

	// No error path, check what to do with this event
	if c.IsLeader() {
		return c.syncProfile(ctx, name, profile)
	}

	// In follower mode, we simply sync updates from Kubernetes
	// If the object is present in Kubernetes, we will update our local version
	// Otherwise, we clear it from our local store
	if profile != nil {
		pi, found, storeUnlock := c.store.LockRead(name, true)
		if found {
			err := pi.UpdateFromProfile(profile)
			if err != nil {
				storeUnlock()
				return autoscaling.Requeue, err
			}
			c.store.UnlockSet(name, pi, c.ID)
		} else {
			pi, err := model.NewPodAutoscalerProfileInternal(profile)
			if err != nil {
				storeUnlock()
				return autoscaling.Requeue, err
			}
			c.store.UnlockSet(name, pi, c.ID)
		}
	} else {
		c.store.Delete(name, c.ID)
	}

	return autoscaling.NoRequeue, nil
}

func (c *Controller) syncProfile(ctx context.Context, name string, profile *datadoghq.DatadogPodAutoscalerClusterProfile) (autoscaling.ProcessResult, error) {
	profileInternal, profileInternalfound, storeUnlock := c.store.LockRead(name, true)

	// Object is missing from our store
	if !profileInternalfound {
		if profile != nil {
			// If we don't have an instance locally, we create it
			log.Debugf("Creating internal PodAutoscalerClusterProfile: %s from Kubernetes object", name)
			profileInternal, err := model.NewPodAutoscalerProfileInternal(profile)
			if err != nil {
				storeUnlock()
				return autoscaling.Requeue, err
			}

			c.store.UnlockSet(name, profileInternal, c.ID)
			return autoscaling.Requeue, nil
		}

		// If podAutoscaler == nil, both objects are nil, nothing to do
		log.Debugf("Reconciling object: %s but object is not present in Kubernetes nor in internal store, nothing to do", name)
		storeUnlock()
		return autoscaling.NoRequeue, nil
	}

	// Object is not present in Kubernetes, we delete it
	if profile == nil {
		c.store.UnlockDelete(name, c.ID)
		return autoscaling.NoRequeue, nil
	}

	// Object is present in both our store and Kubernetes, we need to sync them
	err := profileInternal.UpdateFromProfile(profile)
	return autoscaling.NoRequeue, c.updateAutoscalerStatusAndUnlock(ctx, name, err, profileInternal, profile)
}

func (c *Controller) updateProfileStatus(ctx context.Context, profileInternal model.PodAutoscalerProfileInternal, profile *datadoghq.DatadogPodAutoscalerClusterProfile) error {
	newStatus := profileInternal.BuildStatus(metav1.NewTime(c.clock.Now()), &profile.Status)

	if autoscaling.Semantic.DeepEqual(profile.Status, newStatus) {
		return nil
	}

	log.Debugf("Updating Profile Status: %s", profileInternal.Name())
	profileObj := &datadoghq.DatadogPodAutoscalerClusterProfile{
		TypeMeta:   podAutoscalerClusterProfileMeta,
		ObjectMeta: profile.ObjectMeta,
		Status:     newStatus,
	}

	obj, err := autoscaling.ToUnstructured(profileObj)
	if err != nil {
		return err
	}

	_, err = c.Client.Resource(podAutoscalerClusterProfileGVR).UpdateStatus(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("Unable to update PodAutoscalerProfile Status: %s, err: %w", profileInternal.Name(), err)
	}

	return nil
}

func (c *Controller) updateAutoscalerStatusAndUnlock(ctx context.Context, name string, err error, profileInternal model.PodAutoscalerProfileInternal, profile *datadoghq.DatadogPodAutoscalerClusterProfile) error {
	// Update status based on latest state
	statusErr := c.updateProfileStatus(ctx, profileInternal, profile)
	if statusErr != nil {
		log.Errorf("Failed to update status for Profile: %s, err: %v", profileInternal.Name(), statusErr)

		// We want to return the status error if none to count in the requeue retries.
		if err == nil {
			err = statusErr
		}
	}

	c.store.UnlockSet(name, profileInternal, c.ID)
	return err
}

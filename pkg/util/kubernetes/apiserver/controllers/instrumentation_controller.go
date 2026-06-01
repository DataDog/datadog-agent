// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package controllers

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cenkalti/backoff/v5"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

// isInstrumentationCRDNotFound returns true if the error indicates the DatadogInstrumentation CRD
// is not installed in the cluster (i.e., the API group is not registered).
func isInstrumentationCRDNotFound(err error) bool {
	status, ok := err.(*apierrors.StatusError)
	if !ok {
		return false
	}
	details := status.Status().Details
	return status.Status().Reason == metav1.StatusReasonNotFound &&
		details != nil &&
		details.Group == instrumentation.DatadogInstrumentationGVR.Group
}

func tryCheckInstrumentationCRD(check checkAPI) error {
	if err := check(); err != nil {
		if isInstrumentationCRDNotFound(err) {
			return err
		}
		log.Errorf("DatadogInstrumentation CRD check failed: not retryable: %s", err)
		return backoff.Permanent(err)
	}
	log.Info("DatadogInstrumentation CRD check successful")
	return nil
}

func waitForInstrumentationCRD(ctx context.Context, dynamicClient dynamic.Interface) {
	exp := &backoff.ExponentialBackOff{
		InitialInterval:     crdCheckInitialInterval,
		RandomizationFactor: 0,
		Multiplier:          crdCheckMultiplier,
		MaxInterval:         crdCheckMaxInterval,
	}
	exp.Reset()

	check := func() error {
		_, err := dynamicClient.Resource(instrumentation.DatadogInstrumentationGVR).List(context.TODO(), metav1.ListOptions{})
		return err
	}

	attempt := 0
	_, _ = backoff.Retry(ctx, func() (any, error) {
		err := tryCheckInstrumentationCRD(check)
		if err != nil && isInstrumentationCRDNotFound(err) {
			attempt++
			log.Warnf("DatadogInstrumentation CRD missing (attempt=%d): will retry", attempt)
		}
		return nil, err
	}, backoff.WithBackOff(exp), backoff.WithMaxElapsedTime(crdCheckMaxElapsedTime))
}

// startDatadogInstrumentationController starts the shared DatadogInstrumentation reconciliation controller.
// It waits asynchronously for the DatadogInstrumentation CRD to be installed before starting.
func startDatadogInstrumentationController(ctx *ControllerContext, _ chan error) {
	controllerCtx, cancel := context.WithCancel(context.Background())
	go func() {
		<-ctx.StopCh
		cancel()
	}()

	go func() {
		waitForInstrumentationCRD(controllerCtx, ctx.DynamicClient)

		if controllerCtx.Err() != nil {
			return
		}

		controller, err := instrumentation.NewController(
			ctx.DynamicUpdateClient,
			ctx.DynamicInformerFactory,
			ctx.InstrumentationHandlers,
			ctx.IsLeaderFunc,
		)
		if err != nil {
			log.Errorf("Failed to create DatadogInstrumentation controller: %v", err)
			return
		}

		go controller.Run(controllerCtx)
		ctx.DynamicInformerFactory.Start(ctx.StopCh)
	}()
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package controllers

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cenkalti/backoff/v6"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

func tryCheckInstrumentationCRD(check checkAPI) error {
	if err := check(); err != nil {
		if apierrors.IsUnauthorized(err) || apierrors.IsForbidden(err) {
			log.Errorf("DatadogInstrumentation CRD check failed: not retryable: %s", err)
			return backoff.Permanent(err)
		}
		return err
	}
	return nil
}

func waitForInstrumentationCRD(ctx context.Context, dynamicClient dynamic.Interface) error {
	exp := &backoff.ExponentialBackOff{
		InitialInterval:     crdCheckInitialInterval,
		RandomizationFactor: 0,
		Multiplier:          crdCheckMultiplier,
		MaxInterval:         crdCheckMaxInterval,
	}
	exp.Reset()

	check := func() error {
		_, err := dynamicClient.Resource(instrumentation.DatadogInstrumentationGVR).List(ctx, metav1.ListOptions{})
		return err
	}

	attempt := 0
	_, err := backoff.Retry(ctx, func() (any, error) {
		err := tryCheckInstrumentationCRD(check)
		if err != nil {
			attempt++
			if apierrors.IsNotFound(err) {
				log.Debugf("DatadogInstrumentation CRD missing (attempt=%d): will retry", attempt)
			} else {
				log.Debugf("DatadogInstrumentation CRD check failed transiently (attempt=%d): %v: will retry", attempt, err)
			}
		}
		return nil, err
	}, backoff.WithBackOff(exp), backoff.WithMaxElapsedTime(crdCheckMaxElapsedTime))
	return err
}

// startDatadogInstrumentationController starts the shared DatadogInstrumentation reconciliation controller.
func startDatadogInstrumentationController(ctx *ControllerContext, _ chan error) {
	if !pkgconfigsetup.Datadog().GetBool("admission_controller.enabled") || !pkgconfigsetup.Datadog().GetBool("admission_controller.validation.enabled") {
		log.Info("DatadogInstrumentation controller not starting, admission controller is needed to run.")
		return
	}

	controllerCtx, cancel := context.WithCancel(context.Background())
	go func() {
		<-ctx.StopCh
		cancel()
	}()

	go func() {
		if err := waitForInstrumentationCRD(controllerCtx, ctx.DynamicClient); err != nil {
			log.Infof("DatadogInstrumentation controller will not start: %v", err)
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

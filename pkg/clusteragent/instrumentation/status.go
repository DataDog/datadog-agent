// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package instrumentation

import (
	"context"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"
)

var metaDatadogInstrumentation = metav1.TypeMeta{
	Kind:       "DatadogInstrumentation",
	APIVersion: datadoghq.GroupVersion.String(),
}

func conditionFromHandlerStatus(status HandlerStatus, generation int64) metav1.Condition {
	return metav1.Condition{
		Type:               status.Type,
		Status:             status.Status,
		Reason:             status.Reason,
		Message:            status.Message,
		ObservedGeneration: generation,
	}
}

func updateStatusConditions(ctx context.Context, client dynamic.Interface, cr *datadoghq.DatadogInstrumentation, statuses []HandlerStatus) error {
	if client == nil || cr == nil || len(statuses) == 0 {
		return nil
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latestObj, err := client.Resource(DatadogInstrumentationGVR).Namespace(cr.Namespace).Get(ctx, cr.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}

		latest := &datadoghq.DatadogInstrumentation{}
		if err := UnstructuredIntoDatadogInstrumentation(latestObj, latest); err != nil {
			return err
		}

		for _, status := range statuses {
			if status.Type == "" {
				continue
			}
			condition := conditionFromHandlerStatus(status, latest.Generation)
			meta.SetStatusCondition(&latest.Status.Conditions, condition)
		}

		statusObj := &datadoghq.DatadogInstrumentation{
			TypeMeta: metaDatadogInstrumentation,
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       latest.Namespace,
				Name:            latest.Name,
				ResourceVersion: latest.ResourceVersion,
			},
			Status: latest.Status,
		}
		unstructuredStatusObj := &unstructured.Unstructured{}
		if err := UnstructuredFromDatadogInstrumentation(statusObj, unstructuredStatusObj); err != nil {
			return err
		}
		_, err = client.Resource(DatadogInstrumentationGVR).Namespace(latest.Namespace).UpdateStatus(ctx, unstructuredStatusObj, metav1.UpdateOptions{})
		return err
	})
}

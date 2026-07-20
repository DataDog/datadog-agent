// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package envoygateway

import (
	"context"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var backendGVR = schema.GroupVersionResource{Resource: "backends", Group: "gateway.envoyproxy.io", Version: "v1alpha1"}

// newBackend builds an egv1a1.Backend targeting a Unix domain socket at socketPath.
func newBackend(namespace, socketPath string, labels, annotations map[string]string) egv1a1.Backend {
	return egv1a1.Backend{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "gateway.envoyproxy.io/v1alpha1",
			Kind:       "Backend",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        extProcName,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: egv1a1.BackendSpec{
			Endpoints: []egv1a1.BackendEndpoint{
				{
					Unix: &egv1a1.UnixSocket{Path: socketPath},
				},
			},
		},
	}
}

// createBackend creates the Backend resource in the given namespace.
// It is idempotent: if the resource already exists the call is a no-op.
func (e *envoyGatewayInjectionPattern) createBackend(ctx context.Context, namespace, socketPath string) error {
	backend := newBackend(namespace, socketPath, e.config.CommonLabels, e.config.CommonAnnotations)

	unstructuredBackend, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&backend)
	if err != nil {
		return err
	}

	_, err = e.client.Resource(backendGVR).
		Namespace(namespace).
		Create(ctx, &unstructured.Unstructured{Object: unstructuredBackend}, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		e.logger.Debugf("Backend %q already exists in namespace %s", extProcName, namespace)
		return nil
	}

	if err != nil {
		return err
	}

	e.logger.Infof("Backend %q created in namespace %s", extProcName, namespace)
	return nil
}

// deleteBackend deletes the Backend resource from the given namespace.
// It is idempotent: if the resource does not exist the call is a no-op.
func (e *envoyGatewayInjectionPattern) deleteBackend(ctx context.Context, namespace string) error {
	err := e.client.Resource(backendGVR).
		Namespace(namespace).
		Delete(ctx, extProcName, metav1.DeleteOptions{})
	if errors.IsNotFound(err) {
		e.logger.Debugf("Backend %q already absent from namespace %s", extProcName, namespace)
		return nil
	}

	if err != nil {
		return err
	}

	e.logger.Infof("Backend %q deleted from namespace %s", extProcName, namespace)
	return nil
}

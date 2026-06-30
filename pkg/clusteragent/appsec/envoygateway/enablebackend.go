// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package envoygateway

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	sigsyaml "sigs.k8s.io/yaml"
)

const (
	envoyGatewaySystemNamespace = "envoy-gateway-system"
	envoyGatewayConfigMapName   = "envoy-gateway-config"
	envoyGatewayConfigDataKey   = "envoy-gateway.yaml"
)

var configMapGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}

type egConfig struct {
	ExtensionAPIs struct {
		EnableBackend bool `json:"enableBackend" yaml:"enableBackend"`
	} `json:"extensionApis" yaml:"extensionApis"`
}

func (e *envoyGatewayInjectionPattern) isBackendExtensionEnabled(ctx context.Context, namespace string) (enabled bool, found bool, err error) {
	cm, err := e.client.Resource(configMapGVR).Namespace(namespace).Get(ctx, envoyGatewayConfigMapName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}

	yamlBlob, hasKey, err := unstructured.NestedString(cm.Object, "data", envoyGatewayConfigDataKey)
	if err != nil {
		return false, true, fmt.Errorf("could not read %q from ConfigMap %s/%s: %w", envoyGatewayConfigDataKey, namespace, envoyGatewayConfigMapName, err)
	}
	if !hasKey || yamlBlob == "" {
		return false, false, nil
	}

	var cfg egConfig
	if unmarshalErr := sigsyaml.Unmarshal([]byte(yamlBlob), &cfg); unmarshalErr != nil {
		return false, true, unmarshalErr
	}

	return cfg.ExtensionAPIs.EnableBackend, true, nil
}

func (e *envoyGatewayInjectionPattern) warnIfBackendDisabled(ctx context.Context, namespace string) {
	enabled, found, err := e.isBackendExtensionEnabled(ctx, namespace)
	if err != nil {
		e.logger.Warnf("Failed to check if envoy gateway Backend extension API is enabled in namespace %s: %v", namespace, err)
		return
	}

	if enabled {
		e.logger.Debugf("Envoy gateway Backend extension API is enabled in namespace %s", namespace)
		return
	}

	var msg string
	if !found {
		msg = "ConfigMap " + envoyGatewayConfigMapName + " not found in namespace " + namespace +
			"; set extensionApis.enableBackend: true in " + envoyGatewayConfigMapName + " to enable the Backend extension API"
	} else {
		msg = "ConfigMap " + envoyGatewayConfigMapName + " in namespace " + namespace +
			" has extensionApis.enableBackend set to false; set it to true to enable the Backend extension API"
	}

	e.logger.Warnf("%s", msg)
	e.recorder.Eventf(
		&corev1.ObjectReference{
			Kind:       "ConfigMap",
			Name:       envoyGatewayConfigMapName,
			Namespace:  namespace,
			APIVersion: "v1",
		},
		corev1.EventTypeWarning,
		EventReasonBackendExtensionDisabled,
		msg,
	)
}

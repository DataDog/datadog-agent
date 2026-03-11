// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package checks

import (
	"context"
	"fmt"
	"maps"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Handler implements ConfigSectionHandler for the checks section.
// It converts DatadogWorkloadConfig CRs into AD-compatible check configs
// and writes them to a ConfigMap that the Node Agent reads via file-based AD.
type Handler struct {
	kubeClient         kubernetes.Interface
	configMapName      string
	configMapNamespace string
}

// NewChecksHandler creates a new Handler.
func NewChecksHandler(kubeClient kubernetes.Interface, configMapName, configMapNamespace string) *Handler {
	return &Handler{
		kubeClient:         kubeClient,
		configMapName:      configMapName,
		configMapNamespace: configMapNamespace,
	}
}

// Name returns the handler name.
func (h *Handler) Name() string {
	return "checks"
}

// Reconcile processes all CRs and writes their check configs to the ConfigMap.
func (h *Handler) Reconcile(crs []*datadoghq.DatadogWorkloadConfig) error {
	data := make(map[string]string)
	for _, dwc := range crs {
		entries, err := convertCR(dwc)
		if err != nil {
			log.Warnf("Skipping DatadogWorkloadConfig %s/%s: %v", dwc.Namespace, dwc.Name, err)
			continue
		}

		for k, v := range entries {
			data[k] = v
		}
	}

	return h.updateConfigMap(data)
}

// updateConfigMap updates the ConfigMap only if the data has changed.
func (h *Handler) updateConfigMap(data map[string]string) error {
	cm, err := h.kubeClient.CoreV1().ConfigMaps(h.configMapNamespace).Get(
		context.TODO(), h.configMapName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap %s/%s: %w", h.configMapNamespace, h.configMapName, err)
	}

	if maps.Equal(cm.Data, data) {
		return nil
	}

	cm.Data = data
	_, err = h.kubeClient.CoreV1().ConfigMaps(h.configMapNamespace).Update(
		context.TODO(), cm, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update ConfigMap %s/%s: %w", h.configMapNamespace, h.configMapName, err)
	}

	log.Infof("Updated ConfigMap %s/%s with %d check configs", h.configMapNamespace, h.configMapName, len(data))
	return nil
}

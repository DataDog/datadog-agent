// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// readCAFromFilesystem reads the CA certificate from the cluster-agent filesystem.
// Returns nil if copy_ca_configmap is disabled, the path is not configured, or the file cannot be read.
func readCAFromFilesystem(datadogConfig config.Component) map[string]string {
	if !datadogConfig.GetBool("admission_controller.agent_sidecar.cluster_agent.tls_verification.copy_ca_configmap") {
		return nil
	}

	caCertPath := datadogConfig.GetString("cluster_trust_chain.ca_cert_file_path")
	if caCertPath == "" {
		log.Errorf("CA cert copy enabled but cluster_trust_chain.ca_cert_file_path not configured")
		return nil
	}

	certData, err := os.ReadFile(caCertPath)
	if err != nil {
		log.Errorf("Failed to read CA cert from %s: %v", caCertPath, err)
		return nil
	}

	return map[string]string{
		"ca.crt": string(certData),
	}
}

// ensureCACertConfigMapInNamespace creates or updates the CA cert ConfigMap in the target namespace
func ensureCACertConfigMapInNamespace(namespace string, caCertData map[string]string, client kubernetes.Interface) error {
	// Check if ConfigMap exists
	targetConfigMap, err := client.CoreV1().ConfigMaps(namespace).Get(context.TODO(), configMapCAName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check target ConfigMap: %w", err)
	}

	if apierrors.IsNotFound(err) {
		// Create new ConfigMap
		log.Infof("Creating CA cert ConfigMap in namespace %s", namespace)
		return createCACertConfigMap(namespace, caCertData, client)
	}

	// Update if stale
	if caCertData["ca.crt"] != targetConfigMap.Data["ca.crt"] {
		log.Infof("CA cert ConfigMap in namespace %s is stale, updating", namespace)
		return updateCACertConfigMap(namespace, caCertData, targetConfigMap, client)
	}

	log.Debugf("CA cert ConfigMap in namespace %s is up-to-date", namespace)
	return nil
}

// createCACertConfigMap creates a new ConfigMap with CA cert data
func createCACertConfigMap(namespace string, caCertData map[string]string, client kubernetes.Interface) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapCAName,
			Namespace: namespace,
			Labels: map[string]string{
				"agent.datadoghq.com/configmap-type": "cluster-agent-ca-cert",
			},
		},
		Data: caCertData,
	}

	_, err := client.CoreV1().ConfigMaps(namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
	return err
}

// updateCACertConfigMap updates an existing ConfigMap with new CA cert data
func updateCACertConfigMap(namespace string, caCertData map[string]string, existingConfigMap *corev1.ConfigMap, client kubernetes.Interface) error {
	configMap := existingConfigMap.DeepCopy()
	configMap.Data = caCertData

	_, err := client.CoreV1().ConfigMaps(namespace).Update(context.TODO(), configMap, metav1.UpdateOptions{})
	return err
}

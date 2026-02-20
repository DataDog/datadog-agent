// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/certificate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// readCAFromFilesystem reads certificate authority and key from the cluster-agent filesystem
func readCAFromFilesystem() (map[string][]byte, error) {
	caCertPath := pkgconfigsetup.Datadog().GetString("cluster_trust_chain.ca_cert_file_path")
	caKeyPath := pkgconfigsetup.Datadog().GetString("cluster_trust_chain.ca_key_file_path")

	if caCertPath == "" || caKeyPath == "" {
		return nil, errors.New("cluster_trust_chain paths not configured")
	}

	certData, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert from %s: %w", caCertPath, err)
	}

	keyData, err := os.ReadFile(caKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA key from %s: %w", caKeyPath, err)
	}

	return map[string][]byte{
		"tls.crt": certData,
		"tls.key": keyData,
	}, nil
}

// ensureCASecretInNamespace creates or updates the CA cert secret in the target namespace
func ensureCASecretInNamespace(namespace string, caCertData map[string][]byte, client kubernetes.Interface) error {
	// Check if secret exists
	targetSecret, err := client.CoreV1().Secrets(namespace).Get(context.TODO(), secretCAName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check target secret: %w", err)
	}

	if apierrors.IsNotFound(err) {
		// Create new secret
		log.Infof("Creating CA cert secret in namespace %s", namespace)
		return createCACertSecret(namespace, caCertData, client)
	}

	// Update if stale
	if isCACertSecretStale(caCertData, targetSecret.Data) {
		log.Infof("CA cert secret in namespace %s is stale, updating", namespace)
		return updateCACertSecret(namespace, caCertData, targetSecret, client)
	}

	log.Debugf("CA cert secret in namespace %s is up-to-date", namespace)
	return nil
}

// isCACertSecretStale checks if the target secret is stale compared to the filesystem data
func isCACertSecretStale(filesystemData, targetSecretData map[string][]byte) bool {
	// Compare certificate data from filesystem vs target secret
	if !bytes.Equal(filesystemData["tls.crt"], targetSecretData["tls.crt"]) ||
		!bytes.Equal(filesystemData["tls.key"], targetSecretData["tls.key"]) {
		return true
	}

	// Check expiration of target certificate
	cert, err := certificate.GetCertFromSecret(targetSecretData)
	if err != nil {
		return true // If we can't parse it, consider it stale
	}

	// Consider stale if certificate expires within 30 days
	if certificate.GetDurationBeforeExpiration(cert) < 30*24*time.Hour {
		return true
	}

	return false
}

// createCACertSecret creates a new secret with CA cert data
func createCACertSecret(namespace string, caCertData map[string][]byte, client kubernetes.Interface) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretCAName,
			Namespace: namespace,
			Labels: map[string]string{
				"agent.datadoghq.com/secret-type": "cluster-agent-ca-cert",
			},
		},
		Type: corev1.SecretTypeTLS,
		Data: caCertData,
	}

	_, err := client.CoreV1().Secrets(namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	return err
}

// updateCACertSecret updates an existing secret with new CA cert data
func updateCACertSecret(namespace string, caCertData map[string][]byte, existingSecret *corev1.Secret, client kubernetes.Interface) error {
	secret := existingSecret.DeepCopy()
	secret.Data = caCertData

	_, err := client.CoreV1().Secrets(namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
	return err
}

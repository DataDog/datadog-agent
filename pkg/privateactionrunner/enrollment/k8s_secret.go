// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package enrollment

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	configModel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultSecretName  = "private-action-runner-identity"
	privateKeyField    = "private_key"
	urnField           = "urn"
	secretPollInterval = 1 * time.Second
)

// getIdentityFromK8sSecret retrieves PAR identity from a Kubernetes secret
func getIdentityFromK8sSecret(ctx context.Context, cfg configModel.Reader) (*PersistedIdentity, error) {
	client, err := getKubeClient()
	if err != nil {
		return nil, err
	}

	ns := namespace.GetResourcesNamespace()
	secretName := getSecretName(cfg)

	le, err := leaderelection.GetLeaderEngine()
	if err != nil {
		return nil, err
	}

	secret, err := client.CoreV1().Secrets(ns).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		if isNonTransientK8sError(err) {
			return nil, fmt.Errorf("failed to get identity secret: %w", err)
		}
		if k8serrors.IsNotFound(err) {
			log.Info("PAR identity secret does not exist, waiting for leader to create it...")
		} else {
			log.Warnf("Transient error fetching PAR identity secret, will retry: %v", err)
		}
		leadershipChange, isLeader := le.Subscribe()
		secret, err = waitForLeaderAndSecret(
			ctx, leadershipChange, isLeader, client, ns, secretName,
			secretPollInterval,
		)
		if err != nil {
			return nil, err
		}
	}
	if secret == nil {
		return nil, nil
	}
	return parseSecretData(secret, ns, secretName)
}

// waitForLeaderAndSecret waits until either:
// - We become leader (then returns nil to trigger enrollment)
// - The secret appears (created by current or previous leader)
// - The context is cancelled
//
// Transient K8s API errors (timeouts, rate limiting, 5xx) are logged and
// retried indefinitely at a constant polling interval.
func waitForLeaderAndSecret(
	ctx context.Context,
	leadershipChange <-chan struct{},
	isLeader func() bool,
	client kubernetes.Interface,
	ns, secretName string,
	pollInterval time.Duration,
) (*corev1.Secret, error) {
	if isLeader() {
		log.Info("This replica is the leader, will create PAR identity secret")
		return nil, nil
	}

	log.Info("This replica is a follower, waiting for leader to create PAR identity secret")

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-leadershipChange:
			if isLeader() {
				log.Info("Became leader, will create PAR identity secret")
				return nil, nil
			}

		case <-ticker.C:
			secret, err := client.CoreV1().Secrets(ns).Get(ctx, secretName, metav1.GetOptions{})
			if err == nil {
				log.Infof("Follower replica: found PAR identity secret created by leader: %s/%s", ns, secretName)
				return secret, nil
			}
			if isNonTransientK8sError(err) {
				return nil, fmt.Errorf("non-transient error checking for secret %s/%s: %w", ns, secretName, err)
			}
			if !k8serrors.IsNotFound(err) {
				log.Warnf("Transient error checking for secret %s/%s (will retry): %v", ns, secretName, err)
			}
		}
	}
}

// parseSecretData extracts identity data from a Kubernetes secret
func parseSecretData(secret *corev1.Secret, ns, secretName string) (*PersistedIdentity, error) {
	privateKey, ok := secret.Data[privateKeyField]
	if !ok || len(privateKey) == 0 {
		return nil, errors.New("private_key field is missing or empty in secret")
	}

	urn, ok := secret.Data[urnField]
	if !ok || len(urn) == 0 {
		return nil, errors.New("urn field is missing or empty in secret")
	}

	log.Infof("Loaded PAR identity from K8s secret: %s/%s", ns, secretName)

	return &PersistedIdentity{
		PrivateKey: string(privateKey),
		URN:        string(urn),
	}, nil
}

// persistIdentityToK8sSecret saves the enrollment result to a Kubernetes secret
func persistIdentityToK8sSecret(ctx context.Context, cfg configModel.Reader, result *Result) error {
	le, err := leaderelection.GetLeaderEngine()
	if err != nil {
		return err
	}
	if !le.IsLeader() {
		log.Info("Not leader, skipping PAR identity secret persistence")
		return nil
	}

	log.Info("Leader replica: persisting PAR identity to K8s secret")

	client, err := getKubeClient()
	if err != nil {
		return err
	}

	privateKeyJWK, err := util.EcdsaToJWK(result.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to convert private key to JWK: %w", err)
	}

	marshalledPrivateKey, err := privateKeyJWK.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal private key to JSON: %w", err)
	}

	encodedPrivateKey := base64.RawURLEncoding.EncodeToString(marshalledPrivateKey)

	ns := namespace.GetResourcesNamespace()
	secretName := getSecretName(cfg)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      secretName,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "datadog-cluster-agent",
				"app.kubernetes.io/component":  "private-action-runner",
				"app.kubernetes.io/managed-by": "datadog-cluster-agent",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			privateKeyField: []byte(encodedPrivateKey),
			urnField:        []byte(result.URN),
		},
	}

	_, err = client.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create secret: %w", err)
		}
		// Secret already exists, update it
		_, err = client.CoreV1().Secrets(ns).Update(ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update existing secret: %w", err)
		}
		log.Infof("Updated PAR identity in K8s secret: %s/%s", ns, secretName)
		return nil
	}

	log.Infof("Created PAR identity in K8s secret: %s/%s", ns, secretName)
	return nil
}

// isNonTransientK8sError returns true for errors that indicate a permanent
// problem (e.g. RBAC misconfiguration) that will not resolve by retrying.
// Unknown errors and network-level errors are assumed transient.
func isNonTransientK8sError(err error) bool {
	return k8serrors.IsForbidden(err) ||
		k8serrors.IsUnauthorized(err) ||
		k8serrors.IsBadRequest(err) ||
		k8serrors.IsMethodNotSupported(err) ||
		k8serrors.IsNotAcceptable(err) ||
		k8serrors.IsGone(err) ||
		k8serrors.IsInvalid(err) ||
		k8serrors.IsRequestEntityTooLargeError(err) ||
		k8serrors.IsUnsupportedMediaType(err)
}

func getKubeClient() (kubernetes.Interface, error) {
	client, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes client: %w", err)
	}
	return client.Cl, nil
}

func getSecretName(cfg configModel.Reader) string {
	if secretName := cfg.GetString(setup.PARIdentitySecretName); secretName != "" {
		return secretName
	}
	return defaultSecretName
}

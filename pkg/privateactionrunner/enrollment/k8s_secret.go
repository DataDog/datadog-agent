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
	defaultSecretName      = "private-action-runner-identity"
	privateKeyField        = "private_key"
	urnField               = "urn"
	secretWaitTimeout      = 5 * time.Minute
	secretWaitPollInterval = 5 * time.Second
)

// getIdentityFromK8sSecret retrieves PAR identity from a Kubernetes secret
func getIdentityFromK8sSecret(ctx context.Context, cfg configModel.Reader) (*PersistedIdentity, error) {
	client, err := getKubeClient()
	if err != nil {
		return nil, err
	}

	ns := namespace.GetResourcesNamespace()
	secretName := getSecretName(cfg)

	// Check if we're a follower - if so, we may need to wait for leader to create the secret
	le, err := leaderelection.GetLeaderEngine()
	isFollower := err == nil && !le.IsLeader()

	secret, err := client.CoreV1().Secrets(ns).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// If we're a follower and secret doesn't exist, wait for leader to create it
			if isFollower {
				log.Info("Follower replica: waiting for leader to create PAR identity secret")
				secret, err = waitForSecretCreation(ctx, client, ns, secretName)
				if err != nil {
					return nil, err
				}
				// Continue to parse the secret below
			} else {
				// Leader or no leader election - return nil to trigger self-enrollment
				return nil, nil
			}
		} else {
			return nil, fmt.Errorf("failed to get identity secret: %w", err)
		}
	}

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
	// Check if this replica is the leader before creating the secret
	le, err := leaderelection.GetLeaderEngine()
	if err != nil {
		log.Warnf("Failed to get leader engine, proceeding without leader check: %v", err)
		// Fall through to create secret anyway if leader election is not available
	} else if !le.IsLeader() {
		log.Info("Not leader, skipping PAR identity secret creation (leader will handle it)")
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

// waitForSecretCreation waits for the leader to create the PAR identity secret
func waitForSecretCreation(ctx context.Context, client kubernetes.Interface, ns, secretName string) (*corev1.Secret, error) {
	deadline := time.Now().Add(secretWaitTimeout)
	ticker := time.NewTicker(secretWaitPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled while waiting for secret: %w", ctx.Err())
		case <-ticker.C:
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("timeout waiting for leader to create secret %s/%s after %v", ns, secretName, secretWaitTimeout)
			}

			secret, err := client.CoreV1().Secrets(ns).Get(ctx, secretName, metav1.GetOptions{})
			if err == nil {
				log.Infof("Follower replica: found PAR identity secret created by leader: %s/%s", ns, secretName)
				return secret, nil
			}

			if !k8serrors.IsNotFound(err) {
				return nil, fmt.Errorf("error checking for secret: %w", err)
			}
			// Secret still not found, continue waiting
		}
	}
}

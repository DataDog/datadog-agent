// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package k8sstoreimpl

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	identitystore "github.com/DataDog/datadog-agent/comp/privateactionrunner/identitystore/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Configuration keys for the private action runner.
// These mirror the constants in pkg/config/setup but are defined here
// because comp/ packages cannot import pkg/config/setup (depguard rule).
const (
	parIdentityUseK8sSecret = "private_action_runner.use_k8s_secret"
	parIdentitySecretName   = "private_action_runner.identity_secret_name"
	defaultSecretName       = "private-action-runner-identity"
	privateKeyField         = "private_key"
	urnField                = "urn"
)

// Requires defines the dependencies for the K8s-based identity store
type Requires struct {
	compdef.In

	Config config.Component
	Log    log.Component
}

// Provides defines the output of the K8s-based identity store
type Provides struct {
	compdef.Out

	Comp identitystore.Component
}

type k8sStore struct {
	config    config.Component
	log       log.Component
	client    kubernetes.Interface
	namespace string
}

// NewComponent creates a new Kubernetes secret-based identity store
func NewComponent(reqs Requires) (Provides, error) {
	// Get Kubernetes client
	kubeClient, err := apiserver.GetKubeClient(10*time.Second, 0, 0)
	if err != nil {
		return Provides{}, fmt.Errorf("failed to get Kubernetes client: %w", err)
	}

	ns := namespace.GetResourcesNamespace()

	return Provides{
		Comp: &k8sStore{
			config:    reqs.Config,
			log:       reqs.Log,
			client:    kubeClient,
			namespace: ns,
		},
	}, nil
}

func (k *k8sStore) GetIdentity(ctx context.Context) (*identitystore.Identity, error) {
	secretName := k.getSecretName()

	secret, err := k.client.CoreV1().Secrets(k.namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			k.log.Debugf("Identity secret %s/%s does not exist", k.namespace, secretName)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get identity secret: %w", err)
	}

	privateKey, ok := secret.Data[privateKeyField]
	if !ok || len(privateKey) == 0 {
		return nil, errors.New("private_key field is missing or empty in secret")
	}

	urn, ok := secret.Data[urnField]
	if !ok || len(urn) == 0 {
		return nil, errors.New("urn field is missing or empty in secret")
	}

	k.log.Infof("Loaded PAR identity from K8s secret: %s/%s", k.namespace, secretName)

	return &identitystore.Identity{
		PrivateKey: string(privateKey),
		URN:        string(urn),
	}, nil
}

func (k *k8sStore) PersistIdentity(ctx context.Context, identity *identitystore.Identity) error {
	secretName := k.getSecretName()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: k.namespace,
			Name:      secretName,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "datadog-cluster-agent",
				"app.kubernetes.io/component":  "private-action-runner",
				"app.kubernetes.io/managed-by": "datadog-cluster-agent",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			privateKeyField: []byte(identity.PrivateKey),
			urnField:        []byte(identity.URN),
		},
	}

	// Try to create the secret
	_, err := k.client.CoreV1().Secrets(k.namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create secret: %w", err)
		}
		// Secret already exists, update it
		_, err = k.client.CoreV1().Secrets(k.namespace).Update(ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update existing secret: %w", err)
		}
		k.log.Infof("Updated PAR identity in K8s secret: %s/%s", k.namespace, secretName)
		return nil
	}

	k.log.Infof("Created PAR identity in K8s secret: %s/%s", k.namespace, secretName)
	return nil
}

func (k *k8sStore) DeleteIdentity(ctx context.Context) error {
	secretName := k.getSecretName()

	err := k.client.CoreV1().Secrets(k.namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete identity secret: %w", err)
	}

	k.log.Infof("Deleted identity secret: %s/%s", k.namespace, secretName)
	return nil
}

func (k *k8sStore) getSecretName() string {
	if secretName := k.config.GetString(parIdentitySecretName); secretName != "" {
		return secretName
	}
	return defaultSecretName
}

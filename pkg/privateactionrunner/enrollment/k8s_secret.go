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
	defaultSecretName         = "private-action-runner-identity"
	privateKeyField           = "private_key"
	urnField                  = "urn"
	orchClusterIDField        = "orch_cluster_id"
	apiKeyHashField           = "api_key_hash"
	authorizationTypeField    = "authorization_type"
	mappingIDField            = "intake_mapping_id"
	providerField             = "provider"
	pendingField              = "pending"
	runnerNameField           = "runner_name"
	authorizationVersionField = "authorization_version"
	secretPollInterval        = 1 * time.Second
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
	if (!ok || len(urn) == 0) && !(string(secret.Data[pendingField]) == "true" && string(secret.Data[authorizationTypeField]) == WorkloadIdentityAuthorization) {
		return nil, errors.New("urn field is missing or empty in secret")
	}

	var authorizationVersion int64
	if raw := secret.Data[authorizationVersionField]; len(raw) > 0 {
		if _, err := fmt.Sscan(string(raw), &authorizationVersion); err != nil || authorizationVersion < 0 {
			return nil, errors.New("invalid persisted authorization version")
		}
	}
	log.Infof("Loaded PAR identity from K8s secret: %s/%s", ns, secretName)

	return &PersistedIdentity{
		PrivateKey:           string(privateKey),
		URN:                  string(urn),
		OrchClusterID:        string(secret.Data[orchClusterIDField]),
		APIKeyHash:           string(secret.Data[apiKeyHashField]),
		AuthorizationVersion: authorizationVersion, AuthorizationType: string(secret.Data[authorizationTypeField]), IntakeMappingID: string(secret.Data[mappingIDField]), Provider: string(secret.Data[providerField]), Pending: string(secret.Data[pendingField]) == "true", RunnerName: string(secret.Data[runnerNameField]),
	}, nil
}

// writeIdentitySecret creates or updates the PAR identity K8s secret.
func writeIdentitySecret(ctx context.Context, client kubernetes.Interface, ns, secretName string, result *Result) error {
	privateKeyJWK, err := util.EcdsaToJWK(result.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to convert private key to JWK: %w", err)
	}
	marshalledPrivateKey, err := privateKeyJWK.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal private key to JSON: %w", err)
	}
	encodedPrivateKey := base64.RawURLEncoding.EncodeToString(marshalledPrivateKey)

	labels := map[string]string{
		"app.kubernetes.io/name":       "datadog-cluster-agent",
		"app.kubernetes.io/component":  "private-action-runner",
		"app.kubernetes.io/managed-by": "datadog-cluster-agent",
	}
	data := map[string][]byte{
		privateKeyField:           []byte(encodedPrivateKey),
		urnField:                  []byte(result.URN),
		orchClusterIDField:        []byte(result.OrchClusterID),
		apiKeyHashField:           []byte(result.APIKeyHash),
		authorizationVersionField: []byte(fmt.Sprint(result.AuthorizationVersion)), authorizationTypeField: []byte(result.AuthorizationType), mappingIDField: []byte(result.IntakeMappingID), providerField: []byte(result.Provider), pendingField: []byte(fmt.Sprint(result.Pending)), runnerNameField: []byte(result.RunnerName),
	}

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      secretName,
			Labels:    labels,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	// Bound the apiserver calls so a stalled admission webhook, missing RBAC, or
	// unresponsive apiserver surfaces as an error instead of hanging PAR startup
	// forever on the long-lived parent context.
	opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err = client.CoreV1().Secrets(ns).Create(opCtx, newSecret, metav1.CreateOptions{})
	if err == nil {
		log.Infof("Created PAR identity in K8s secret: %s/%s", ns, secretName)
		return nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create secret %s/%s: %w", ns, secretName, err)
	}

	// Fetch the live object so Update carries its ResourceVersion.
	existing, err := client.CoreV1().Secrets(ns).Get(opCtx, secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get existing secret: %w", err)
	}
	if result.AuthorizationType == WorkloadIdentityAuthorization && string(existing.Data[privateKeyField]) != encodedPrivateKey {
		return errors.New("refusing to replace a different shared runner key")
	}
	if result.AuthorizationType == WorkloadIdentityAuthorization {
		saved, err := parseSecretData(existing, ns, secretName)
		if err != nil {
			return err
		}
		if saved.AuthorizationVersion > result.AuthorizationVersion || (saved.AuthorizationVersion == result.AuthorizationVersion && !saved.Pending && (saved.IntakeMappingID != result.IntakeMappingID || saved.Provider != result.Provider)) {
			return errors.New("refusing to overwrite newer shared runner authorization")
		}
	}
	existing.Type = corev1.SecretTypeOpaque
	existing.Data = data
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	for k, v := range labels {
		existing.Labels[k] = v
	}
	if _, err = client.CoreV1().Secrets(ns).Update(opCtx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update existing secret: %w", err)
	}
	log.Infof("Updated PAR identity in K8s secret: %s/%s", ns, secretName)
	return nil
}

// persistIdentityToK8sSecret saves the enrollment result to a Kubernetes secret.
func persistIdentityToK8sSecret(ctx context.Context, cfg configModel.Reader, result *Result) error {
	le, err := leaderelection.GetLeaderEngine()
	if err != nil {
		return err
	}
	if !le.IsLeader() {
		if result.AuthorizationType == WorkloadIdentityAuthorization {
			return errors.New("leadership lost before workload identity persistence")
		}
		log.Info("Not leader, skipping PAR identity secret persistence")
		return nil
	}
	log.Info("Leader replica: persisting PAR identity to K8s secret")
	client, err := getKubeClient()
	if err != nil {
		return err
	}
	ns := namespace.GetResourcesNamespace()
	return writeIdentitySecret(ctx, client, ns, getSecretName(cfg), result)
}

// persistIdentityToK8sSecretNoLeader persists identity to K8s secret without requiring leadership.
func persistIdentityToK8sSecretNoLeader(ctx context.Context, cfg configModel.Reader, result *Result) error {
	client, err := getKubeClient()
	if err != nil {
		return err
	}
	ns := namespace.GetResourcesNamespace()
	return writeIdentitySecret(ctx, client, ns, getSecretName(cfg), result)
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

func isWIFLeader() (bool, error) {
	le, err := leaderelection.GetLeaderEngine()
	if err != nil {
		return false, err
	}
	return le.IsLeader(), nil
}
func claimPendingK8sIdentity(ctx context.Context, cfg configModel.Reader, result *Result) (*PersistedIdentity, error) {
	leader, err := isWIFLeader()
	if err != nil {
		return nil, err
	}
	if !leader {
		return nil, errors.New("leadership lost before pending identity persistence")
	}
	client, err := getKubeClient()
	if err != nil {
		return nil, err
	}
	return claimPendingSecret(ctx, client, namespace.GetResourcesNamespace(), getSecretName(cfg), result)
}
func claimPendingSecret(ctx context.Context, client kubernetes.Interface, ns, name string, result *Result) (*PersistedIdentity, error) {
	key, err := util.EcdsaToJWK(result.PrivateKey)
	if err != nil {
		return nil, err
	}
	raw, err := key.MarshalJSON()
	if err != nil {
		return nil, err
	}
	secret, err := client.CoreV1().Secrets(ns).Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Type: corev1.SecretTypeOpaque, Data: map[string][]byte{privateKeyField: []byte(base64.RawURLEncoding.EncodeToString(raw)), urnField: []byte(""), orchClusterIDField: []byte(result.OrchClusterID), authorizationTypeField: []byte(WorkloadIdentityAuthorization), pendingField: []byte("true"), runnerNameField: []byte(result.RunnerName)}}, metav1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		secret, err = client.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
	}
	if err != nil {
		return nil, err
	}
	return parseSecretData(secret, ns, name)
}

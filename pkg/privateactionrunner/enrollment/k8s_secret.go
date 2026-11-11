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
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultSecretName  = "private-action-runner-identity"
	privateKeyField    = "private_key"
	urnField           = "urn"
	orchClusterIDField = "orch_cluster_id"
	apiKeyHashField    = "api_key_hash"
	wifOrgUUIDField    = "wif_org_uuid"
	wifSiteField       = "wif_site"
	secretPollInterval = 1 * time.Second
	// staleSecretWarnTicks spaces the warning about a mismatched secret to once per ~30s.
	staleSecretWarnTicks = 30
)

var errNoLeaderSource = errors.New("leader election is not available")

// k8sIdentityStorage stores the PAR identity in a Kubernetes secret shared by the Cluster Agent replicas.
type k8sIdentityStorage struct {
	kubeClient   func() (kubernetes.Interface, error)
	namespace    string
	secretName   string
	leader       LeaderSource
	pollInterval time.Duration
	apiKeyHash   string
	selfEnroll   bool
}

func newK8sIdentityStorage(cfg configModel.Reader, leader LeaderSource) *k8sIdentityStorage {
	return &k8sIdentityStorage{
		kubeClient:   getKubeClient,
		namespace:    namespace.GetResourcesNamespace(),
		secretName:   getSecretName(cfg),
		leader:       leader,
		pollInterval: secretPollInterval,
		apiKeyHash:   HashAPIKey(cfg.GetString("api_key")),
		selfEnroll:   cfg.GetBool(par.SelfEnroll),
	}
}

// NewK8sIdentityStorage returns the secret storage backed by the given client instead of the apiserver client.
func NewK8sIdentityStorage(cfg configModel.Reader, leader LeaderSource, client kubernetes.Interface, pollInterval time.Duration) IdentityStorage {
	s := newK8sIdentityStorage(cfg, leader)
	s.kubeClient = func() (kubernetes.Interface, error) { return client, nil }
	s.pollInterval = pollInterval
	return s
}

// acceptsSecret reports whether the secret's identity can be used with the configured api_key.
func (s *k8sIdentityStorage) acceptsSecret(secret *corev1.Secret) bool {
	hash := string(secret.Data[apiKeyHashField])
	return hash == "" || hash == s.apiKeyHash
}

// Load retrieves the PAR identity from the Kubernetes secret
func (s *k8sIdentityStorage) Load(ctx context.Context) (*PersistedIdentity, error) {
	client, err := s.kubeClient()
	if err != nil {
		return nil, err
	}
	if s.leader == nil {
		return nil, errNoLeaderSource
	}

	secret, err := client.CoreV1().Secrets(s.namespace).Get(ctx, s.secretName, metav1.GetOptions{})
	switch {
	case err == nil && s.acceptsSecret(secret):
		return parseSecretData(secret, s.namespace, s.secretName)
	case err == nil && !s.selfEnroll:
		log.Warn("PAR identity secret was enrolled with a different api_key and self-enrollment is disabled, ignoring it")
		return nil, nil
	case err == nil:
		log.Info("PAR identity secret was enrolled with a different api_key, waiting for the leader to re-enroll...")
	case isNonTransientK8sError(err):
		return nil, fmt.Errorf("failed to get identity secret: %w", err)
	case k8serrors.IsNotFound(err):
		log.Info("PAR identity secret does not exist, waiting for leader to create it...")
	default:
		log.Warnf("Transient error fetching PAR identity secret, will retry: %v", err)
	}

	leadershipChange, isLeader := s.leader.Subscribe()
	secret, err = waitForLeaderAndSecret(
		ctx, leadershipChange, isLeader, client, s.namespace, s.secretName,
		s.pollInterval, s.acceptsSecret,
	)
	if err != nil {
		return nil, err
	}
	if secret == nil {
		return nil, nil
	}
	return parseSecretData(secret, s.namespace, s.secretName)
}

// waitForLeaderAndSecret waits until either:
// - We become leader (then returns nil to trigger enrollment)
// - A secret accepted by accept appears (created by current or previous leader)
// - The context is cancelled
//
// Leadership is re-checked on every poll so a missed notification cannot block forever.
// Transient K8s API errors (timeouts, rate limiting, 5xx) are logged and
// retried indefinitely at a constant polling interval.
func waitForLeaderAndSecret(
	ctx context.Context,
	leadershipChange <-chan struct{},
	isLeader func() bool,
	client kubernetes.Interface,
	ns, secretName string,
	pollInterval time.Duration,
	accept func(*corev1.Secret) bool,
) (*corev1.Secret, error) {
	if isLeader() {
		log.Info("This replica is the leader, will create PAR identity secret")
		return nil, nil
	}

	log.Info("This replica is a follower, waiting for leader to create PAR identity secret")

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	staleTicks := 0

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
			if isLeader() {
				log.Info("Became leader, will create PAR identity secret")
				return nil, nil
			}
			secret, err := client.CoreV1().Secrets(ns).Get(ctx, secretName, metav1.GetOptions{})
			if err == nil {
				if accept(secret) {
					log.Infof("Follower replica: found PAR identity secret created by leader: %s/%s", ns, secretName)
					return secret, nil
				}
				if staleTicks%staleSecretWarnTicks == 0 {
					log.Warnf("PAR identity secret %s/%s was enrolled with a different api_key, still waiting for the leader to re-enroll", ns, secretName)
				}
				staleTicks++
				continue
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
		PrivateKey:    string(privateKey),
		URN:           string(urn),
		OrchClusterID: string(secret.Data[orchClusterIDField]),
		APIKeyHash:    string(secret.Data[apiKeyHashField]),
		WIFOrgUUID:    string(secret.Data[wifOrgUUIDField]),
		WIFSite:       string(secret.Data[wifSiteField]),
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
		privateKeyField:    []byte(encodedPrivateKey),
		urnField:           []byte(result.URN),
		orchClusterIDField: []byte(result.OrchClusterID),
		apiKeyHashField:    []byte(result.APIKeyHash),
	}
	if result.WIFOrgUUID != "" {
		data[wifOrgUUIDField] = []byte(result.WIFOrgUUID)
		data[wifSiteField] = []byte(result.WIFSite)
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

// Persist saves the enrollment result to the Kubernetes secret when this replica is the leader.
func (s *k8sIdentityStorage) Persist(ctx context.Context, result *Result) error {
	if s.leader == nil {
		return errNoLeaderSource
	}
	if !s.leader.IsLeader() {
		log.Info("Not leader, skipping PAR identity secret persistence")
		return nil
	}
	log.Info("Leader replica: persisting PAR identity to K8s secret")
	client, err := s.kubeClient()
	if err != nil {
		return err
	}
	return writeIdentitySecret(ctx, client, s.namespace, s.secretName, result)
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
	if secretName := cfg.GetString(par.IdentitySecretName); secretName != "" {
		return secretName
	}
	return defaultSecretName
}

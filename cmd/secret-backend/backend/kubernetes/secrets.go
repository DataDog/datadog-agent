// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// Package kubernetes allows to fetch secrets from Kubernetes Secrets API using REST.
package kubernetes

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/mitchellh/mapstructure"

	"github.com/DataDog/datadog-secret-backend/secret"
)

const (
	// Default paths for Kubernetes service account token and CA certificate
	// https://kubernetes.io/docs/concepts/windows/intro/#api
	defaultTokenPathLinux   = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	defaultTokenPathWindows = `C:\var\run\secrets\kubernetes.io\serviceaccount\token`
	defaultCAPathLinux      = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	defaultCAPathWindows    = `C:\var\run\secrets\kubernetes.io\serviceaccount\ca.crt`
)

// SecretsBackendConfig is the configuration for a Kubernetes Secrets backend
type SecretsBackendConfig struct {
	TokenPath string `mapstructure:"token_path"` // optional
	CAPath    string `mapstructure:"ca_path"`    // optional
	APIServer string `mapstructure:"api_server"` // optional
}

// k8sConfig holds the Kubernetes connection configuration
type k8sConfig struct {
	Host        string
	BearerToken string
	CA          []byte
}

// k8sSecretResponse represents the JSON response from K8s API
// https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/secret-v1/#Secret
type k8sSecretResponse struct {
	Kind       string                 `json:"kind"`
	APIVersion string                 `json:"apiVersion"`
	Metadata   map[string]interface{} `json:"metadata"`
	Data       map[string][]byte      `json:"data"`
	Immutable  *bool                  `json:"immutable,omitempty"`
	Type       string                 `json:"type,omitempty"`
}

// k8sErrorResponse represents a simplified expected error responses from K8s API Status object (not-guaranteed)
// https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/status/
type k8sStatusResponse struct {
	Kind       string                 `json:"kind"`
	APIVersion string                 `json:"apiVersion"`
	Status     string                 `json:"status"`
	Message    string                 `json:"message"`
	Reason     string                 `json:"reason"`
	Code       int32                  `json:"code"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

// SecretsBackend represents backend for Kubernetes Secrets
type SecretsBackend struct {
	Config     SecretsBackendConfig
	HTTPClient *http.Client
	K8sConfig  k8sConfig
}

// NewSecretsBackend returns a new Kubernetes Secrets backend
// https://kubernetes.io/docs/tasks/run-application/access-api-from-pod/#directly-accessing-the-rest-api
// https://github.com/kubernetes/client-go/blob/master/rest/config.go#L543
func NewSecretsBackend(bc map[string]interface{}) (*SecretsBackend, error) {
	backendConfig := SecretsBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	tokenPath := backendConfig.TokenPath
	if tokenPath == "" {
		if runtime.GOOS == "windows" {
			tokenPath = defaultTokenPathWindows
		} else {
			tokenPath = defaultTokenPathLinux
		}
	}

	caPath := backendConfig.CAPath
	if caPath == "" {
		if runtime.GOOS == "windows" {
			caPath = defaultCAPathWindows
		} else {
			caPath = defaultCAPathLinux
		}
	}

	token, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read ServiceAccount token: %w", err)
	}

	ca, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	apiServer := backendConfig.APIServer
	if apiServer == "" {
		host := os.Getenv("KUBERNETES_SERVICE_HOST")
		port := os.Getenv("KUBERNETES_SERVICE_PORT")
		if host == "" || port == "" {
			return nil, fmt.Errorf("K8s port and/or host configuration missing")
		}
		apiServer = fmt.Sprintf("https://%s:%s", host, port)
	}

	k8sConfig := &k8sConfig{
		Host:        apiServer,
		BearerToken: string(token),
		CA:          ca,
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(k8sConfig.CA) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	backend := &SecretsBackend{
		Config: backendConfig,
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: caCertPool,
				},
			},
		},
		K8sConfig: *k8sConfig,
	}
	return backend, nil
}

// GetSecretOutput retrieves a secret from Kubernetes Secrets
func (b *SecretsBackend) GetSecretOutput(ctx context.Context, secretString string) secret.Output {
	// https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/secret-v1/#Secret
	// parse: namespace/secret-name;key
	// ex: secrets-store-1/my-secret;password

	parts := strings.SplitN(secretString, ";", 2)
	if len(parts) != 2 {
		es := "invalid secret format, expected 'namespace/secret-name;key'"
		return secret.Output{Value: nil, Error: &es}
	}

	secretPath, secretKey := parts[0], parts[1]

	pathParts := strings.SplitN(secretPath, "/", 2)
	if len(pathParts) != 2 {
		es := "invalid secret format, expected 'namespace/secret-name;key'"
		return secret.Output{Value: nil, Error: &es}
	}

	namespace, secretName := pathParts[0], pathParts[1]

	if namespace == "" || secretName == "" || secretKey == "" {
		es := "namespace, secret name, and key cannot be empty"
		return secret.Output{Value: nil, Error: &es}
	}

	// https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/secret-v1/
	url := fmt.Sprintf("%s/api/v1/namespaces/%s/secrets/%s", b.K8sConfig.Host, namespace, secretName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		es := fmt.Sprintf("failed to create request: %s", err.Error())
		return secret.Output{Value: nil, Error: &es}
	}
	req.Header.Set("Authorization", "Bearer "+b.K8sConfig.BearerToken)
	req.Header.Set("Accept", "application/json")

	resp, err := b.HTTPClient.Do(req)
	if err != nil {
		es := fmt.Sprintf("failed to get secret '%s' in namespace '%s': %s", secretName, namespace, err.Error())
		return secret.Output{Value: nil, Error: &es}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		es := fmt.Sprintf("failed to read response: %s", err.Error())
		return secret.Output{Value: nil, Error: &es}
	}

	if resp.StatusCode != http.StatusOK {
		var k8sErr k8sStatusResponse
		if err := json.Unmarshal(body, &k8sErr); err == nil && k8sErr.Message != "" {
			es := k8sErr.Message
			return secret.Output{Value: nil, Error: &es}
		}
		// fallback error if not an expected status object
		es := fmt.Sprintf("failed to get secret '%s' in namespace '%s': HTTP %d", secretName, namespace, resp.StatusCode)
		return secret.Output{Value: nil, Error: &es}
	}

	var k8sSecret k8sSecretResponse
	if err := json.Unmarshal(body, &k8sSecret); err != nil {
		es := fmt.Sprintf("failed to parse secret response: %s", err.Error())
		return secret.Output{Value: nil, Error: &es}
	}

	if k8sSecret.Data == nil {
		es := fmt.Sprintf("secret '%s' in namespace '%s' has no data", secretName, namespace)
		return secret.Output{Value: nil, Error: &es}
	}

	secretValue, ok := k8sSecret.Data[secretKey]
	if !ok {
		es := secret.ErrKeyNotFound.Error()
		return secret.Output{Value: nil, Error: &es}
	}

	value := string(secretValue)
	return secret.Output{Value: &value, Error: nil}
}

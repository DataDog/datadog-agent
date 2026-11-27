// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package utils

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
)

// DefaultImagePullSecretName is the Kubernetes name used by NewImagePullSecret.
const DefaultImagePullSecretName = "registry-credentials"

// KubeConfigYAMLToJSON safely converts a yaml kubeconfig to a json string.
func KubeConfigYAMLToJSON(kubeConfig pulumi.StringOutput) pulumi.StringInput {
	return kubeConfig.ApplyT(func(config string) (string, error) {
		var body map[string]interface{}
		err := yaml.Unmarshal([]byte(config), &body)
		if err != nil {
			return "", err
		}

		jsonConfig, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		return string(jsonConfig), nil
	}).(pulumi.StringInput)
}

// ImagePullCredential is the credential of one registry of the imagePull parameters.
type ImagePullCredential struct {
	Registry string
	Username string
	Password string
}

// ParseImagePullCredentials parses the comma-separated lists of registries, usernames and
// passwords (as provided by the imagePullRegistry, imagePullUsername and imagePullPassword
// parameters). Passwords prefixed with "b64=" are base64-decoded. It fails when the lists do not
// have the same length, so every consumer of these parameters applies the same validation.
func ParseImagePullCredentials(registry, username, password string) ([]ImagePullCredential, error) {
	registries := strings.Split(registry, ",")
	usernames := strings.Split(username, ",")
	passwords := strings.Split(password, ",")
	if len(registries) != len(usernames) || len(registries) != len(passwords) {
		return nil, fmt.Errorf("the number of registries, usernames, and passwords must be the same")
	}

	credentials := make([]ImagePullCredential, len(registries))
	for i := range registries {
		pwd, err := decodeImagePullPassword(passwords[i])
		if err != nil {
			return nil, fmt.Errorf("registry %s: %w", registries[i], err)
		}
		credentials[i] = ImagePullCredential{Registry: registries[i], Username: usernames[i], Password: pwd}
	}
	return credentials, nil
}

// decodeImagePullPassword returns the image pull password, base64-decoding it when it is
// prefixed with "b64=".
func decodeImagePullPassword(password string) (string, error) {
	encoded, ok := strings.CutPrefix(password, "b64=")
	if !ok {
		return password, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to base64 decode the image pull password: %w", err)
	}
	return string(decoded), nil
}

// DockerConfigJSON builds the content of a kubernetes.io/dockerconfigjson secret from
// comma-separated lists of registries, usernames and passwords (as provided by the
// imagePullRegistry, imagePullUsername and imagePullPassword parameters).
// Passwords prefixed with "b64=" are base64-decoded.
func DockerConfigJSON(registry, username, password string) (string, error) {
	credentials, err := ParseImagePullCredentials(registry, username, password)
	if err != nil {
		return "", err
	}

	authMap := make(map[string]map[string]string)
	for _, credential := range credentials {
		authMap[credential.Registry] = map[string]string{
			"username": credential.Username,
			"password": credential.Password,
			"auth":     base64.StdEncoding.EncodeToString([]byte(credential.Username + ":" + credential.Password)),
		}
	}
	dockerConfigJSON, err := json.Marshal(map[string]map[string]map[string]string{
		"auths": authMap,
	})
	return string(dockerConfigJSON), err
}

// NewImagePullSecret creates an image pull secret based on environment
func NewImagePullSecret(e config.Env, namespace string, opts ...pulumi.ResourceOption) (*corev1.Secret, error) {
	return NewImagePullSecretWithName(e, namespace, DefaultImagePullSecretName, opts...)
}

// NewImagePullSecretWithName creates a named image pull secret based on environment.
func NewImagePullSecretWithName(e config.Env, namespace, name string, opts ...pulumi.ResourceOption) (*corev1.Secret, error) {
	registry := e.ImagePullRegistry()
	username := e.ImagePullUsername()

	dockerConfigJSON := e.ImagePullPassword().ApplyT(func(password string) (string, error) {
		return DockerConfigJSON(registry, username, password)
	}).(pulumi.StringOutput)

	// Pulumi resource name must be unique per namespace to avoid duplicate URNs.
	pulumiName := fmt.Sprintf("%s-%s", name, namespace)
	return corev1.NewSecret(
		e.Ctx(),
		pulumiName,
		&corev1.SecretArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: pulumi.StringPtr(namespace),
				Name:      pulumi.StringPtr(name),
			},
			StringData: pulumi.StringMap{
				".dockerconfigjson": dockerConfigJSON,
			},
			Type: pulumi.StringPtr("kubernetes.io/dockerconfigjson"),
		},
		opts...,
	)
}

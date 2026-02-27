// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"fmt"
	"strings"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
)

const imagePullSecretName = "registry-credentials"

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

// NewImagePullSecret creates an image pull secret based on environment
func NewImagePullSecret(e config.Env, namespace string, opts ...pulumi.ResourceOption) (*corev1.Secret, error) {
	registries := strings.Split(e.ImagePullRegistry(), ",")
	usernames := strings.Split(e.ImagePullUsername(), ",")

	dockerConfigJSON := e.ImagePullPassword().ApplyT(func(password string) (string, error) {
		passwords := strings.Split(password, ",")
		if len(registries) != len(usernames) || len(registries) != len(passwords) {
			return "", fmt.Errorf("the number of registries, usernames, and passwords must be the same")
		}

		authMap := make(map[string]map[string]string)
		for i := range registries {
			authMap[registries[i]] = map[string]string{
				"username": usernames[i],
				"password": passwords[i],
			}
		}
		dockerConfigJSON, err := json.Marshal(map[string]map[string]map[string]string{
			"auths": authMap,
		})
		return string(dockerConfigJSON), err
	}).(pulumi.StringOutput)

	return corev1.NewSecret(
		e.Ctx(),
		imagePullSecretName,
		&corev1.SecretArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: pulumi.StringPtr(namespace),
				Name:      pulumi.StringPtr(imagePullSecretName),
			},
			StringData: pulumi.StringMap{
				".dockerconfigjson": dockerConfigJSON,
			},
			Type: pulumi.StringPtr("kubernetes.io/dockerconfigjson"),
		},
		opts...,
	)
}

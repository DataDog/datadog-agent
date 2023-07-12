// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets

package providers

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	s "github.com/DataDog/datadog-agent/pkg/secrets"
)

// ReadKubernetesSecret TODO <agent-core>
func ReadKubernetesSecret(kubeClient kubernetes.Interface, path string) s.Secret {
	splitName := strings.Split(path, "/")

	if len(splitName) != 3 {
		return s.Secret{ErrorMsg: fmt.Sprintf("invalid format. Use: \"namespace/name/key\"")}
	}

	namespace, name, key := splitName[0], splitName[1], splitName[2]

	secret, err := kubeClient.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return s.Secret{ErrorMsg: err.Error()}
	}

	value, ok := secret.Data[key]
	if !ok {
		return s.Secret{ErrorMsg: fmt.Sprintf("key %s not found in secret %s/%s", key, namespace, name)}
	}

	return s.Secret{Value: string(value)}
}

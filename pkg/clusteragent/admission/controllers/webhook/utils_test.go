// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package webhook

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type fixture struct {
	t      *testing.T //nolint:structcheck
	client *fake.Clientset
}

func (f *fixture) populateSecretsCache(secrets ...*corev1.Secret) {
	for _, s := range secrets {
		_, _ = f.client.CoreV1().Secrets(s.Namespace).Create(context.TODO(), s, metav1.CreateOptions{})
	}
}

func buildSecret(data map[string][]byte, cfg Config) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfg.getSecretNs(),
			Name:      cfg.getSecretName(),
		},
		Data: data,
	}
}

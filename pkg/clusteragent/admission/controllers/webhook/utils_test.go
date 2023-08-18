// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package webhook

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type fixture struct {
	t      *testing.T //nolint:structcheck
	client *fake.Clientset
}

// waitOnActions can be used to wait for controller to start watching
// resources effectively and handling objects before returning it.
// Otherwise tests will start making assertions before the reconciliation is done.
func (f *fixture) waitOnActions() {
	lastChange := time.Now()
	lastCount := 0
	for {
		time.Sleep(1 * time.Second)
		count := len(f.client.Actions())
		if count > lastCount {
			lastChange = time.Now()
			lastCount = count
		} else if time.Since(lastChange) > 2*time.Second {
			return
		}
	}
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

func resetMockConfig(c *config.MockConfig) {
	c.Set("admission_controller.mutate_unlabelled", false)
	c.Set("admission_controller.inject_config.enabled", true)
	c.Set("admission_controller.inject_tags.enabled", true)
	c.Set("admission_controller.namespace_selector_fallback", false)
	c.Set("admission_controller.add_aks_selectors", false)
}

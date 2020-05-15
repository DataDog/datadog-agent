// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package webhook

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/certificate"

	admiv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	cfg = Config{
		name:       "foo",
		secretNs:   "bar",
		secretName: "baz",
		templates: []admiv1beta1.MutatingWebhook{
			{
				Name: "webhook-foo",
			},
			{
				Name: "webhook-bar",
			},
		},
	}
)

func TestSecretNotFound(t *testing.T) {
	f := newFixture(t)
	c := f.run(t)

	_, err := c.webhooksLister.Get(cfg.GetName())
	if !errors.IsNotFound(err) {
		t.Fatal("Webhook shouldn't be created")
	}

	if c.queue.Len() != 0 {
		t.Fatal("Work queue isn't empty")
	}
}

func TestCreateWebhook(t *testing.T) {
	f := newFixture(t)

	data, err := certificate.GenerateSecretData(time.Now(), time.Now().Add(365*24*time.Hour), []string{"my.svc.dns"})
	if err != nil {
		t.Fatalf("Failed to create the Secret: %v", err)
	}

	secret := buildSecret(data)
	f.populateSecretsCache(secret)

	c := f.run(t)

	webhook, err := c.webhooksLister.Get(cfg.GetName())
	if err != nil {
		t.Fatalf("Failed to get the Webhook: %v", err)
	}

	if err := validate(webhook, secret); err != nil {
		t.Fatalf("Invalid Webhook: %v", err)
	}

	if c.queue.Len() != 0 {
		t.Fatal("Work queue isn't empty")
	}
}

func TestUpdateOutdatedWebhook(t *testing.T) {
	f := newFixture(t)

	data, err := certificate.GenerateSecretData(time.Now(), time.Now().Add(365*24*time.Hour), []string{"my.svc.dns"})
	if err != nil {
		t.Fatalf("Failed to create the Secret: %v", err)
	}

	secret := buildSecret(data)
	f.populateSecretsCache(secret)

	webhook := buildWebhook()
	f.populateWebhooksCache(webhook)

	c := f.run(t)

	newWebhook, err := c.webhooksLister.Get(cfg.GetName())
	if err != nil {
		t.Fatalf("Failed to get the Webhook: %v", err)
	}

	if reflect.DeepEqual(webhook, newWebhook) {
		t.Fatal("The Webhook hasn't been modified")
	}

	if err := validate(newWebhook, secret); err != nil {
		t.Fatalf("Invalid Webhook: %v", err)
	}

	if c.queue.Len() != 0 {
		t.Fatal("Work queue isn't empty")
	}
}

func validate(w *admiv1beta1.MutatingWebhookConfiguration, s *corev1.Secret) error {
	if len(w.Webhooks) != 2 {
		return fmt.Errorf("Webhooks should contain 2 entries, got %d", len(w.Webhooks))
	}
	for i := 0; i < len(w.Webhooks); i++ {
		if !reflect.DeepEqual(w.Webhooks[i].ClientConfig.CABundle, certificate.GetCABundle(s.Data)) {
			return fmt.Errorf("The Webhook CABundle doesn't match the Secret: CABundle: %v, Secret: %v", w.Webhooks[i].ClientConfig.CABundle, s)
		}
	}
	return nil
}

type fixture struct {
	t      *testing.T
	client *fake.Clientset
}

func newFixture(t *testing.T) *fixture {
	return &fixture{
		t:      t,
		client: fake.NewSimpleClientset(),
	}
}

func (f *fixture) run(t *testing.T) *Controller {
	stopCh := make(chan struct{})
	defer close(stopCh)

	factory := informers.NewSharedInformerFactory(f.client, time.Duration(0))
	c := NewController(
		f.client,
		factory.Core().V1().Secrets(),
		factory.Admissionregistration().V1beta1().MutatingWebhookConfigurations(),
		func() bool { return true },
		cfg,
	)

	factory.Start(stopCh)
	go c.Run(stopCh)

	// Wait for controller to start watching resources effectively and handling objects
	// before returning it.
	// Otherwise tests will start making assertions before the reconciliation is done.
	lastChange := time.Now()
	lastCount := 0
	for {
		time.Sleep(1 * time.Second)
		count := len(f.client.Actions())
		if count > lastCount {
			lastChange = time.Now()
			lastCount = count
		} else if time.Since(lastChange) > 2*time.Second {
			break
		}
	}

	return c
}

func (f *fixture) populateSecretsCache(secrets ...*corev1.Secret) {
	for _, s := range secrets {
		_, _ = f.client.CoreV1().Secrets(s.Namespace).Create(s)
	}
}

func (f *fixture) populateWebhooksCache(webhooks ...*admiv1beta1.MutatingWebhookConfiguration) {
	for _, w := range webhooks {
		_, _ = f.client.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Create(w)
	}
}

func buildSecret(data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfg.GetSecretNs(),
			Name:      cfg.GetSecretName(),
		},
		Data: data,
	}
}

func buildWebhook() *admiv1beta1.MutatingWebhookConfiguration {
	return &admiv1beta1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfg.GetName(),
		},
		Webhooks: cfg.GetTemplates(),
	}
}

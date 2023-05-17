// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package secret

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/certificate"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	cfg = Config{
		ns:   "foo",
		name: "bar",
		svc:  "baz",
		cert: CertConfig{
			expirationThreshold: 30 * 24 * time.Hour,
			validityBound:       365 * 24 * time.Hour,
		},
	}

	waitFor = 10 * time.Second
	tick    = 10 * time.Millisecond
)

func TestCreateSecret(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	f := newFixture(t)
	c := f.run(stopCh)

	assert.Eventually(t, func() bool {
		_, err := c.secretsLister.Secrets(cfg.GetNs()).Get(cfg.GetName())
		return err == nil && c.queue.Len() == 0
	}, waitFor, tick, "Failed to get the secret")

	secret, _ := c.secretsLister.Secrets(cfg.GetNs()).Get(cfg.GetName())

	if err := validate(secret); err != nil {
		t.Fatalf("Invalid Secret: %v", err)
	}
}

func TestRefreshNotRequired(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	f := newFixture(t)

	// Create a Secret with a valid certificate
	data, err := certificate.GenerateSecretData(time.Now(), time.Now().Add(365*24*time.Hour), generateDNSNames(cfg.GetNs(), cfg.GetSvc()))
	if err != nil {
		t.Fatalf("Failed to create the Secret: %v", err)
	}

	oldSecret := buildSecret(data)
	f.populateCache(oldSecret)

	c := f.run(stopCh)

	assert.Eventually(t, func() bool {
		newSecret, err := c.secretsLister.Secrets(cfg.GetNs()).Get(cfg.GetName())
		return err == nil && reflect.DeepEqual(oldSecret, newSecret) && c.queue.Len() == 0
	}, waitFor, tick, "The secret has been modified")

	newSecret, _ := c.secretsLister.Secrets(cfg.GetNs()).Get(cfg.GetName())

	if err := validate(newSecret); err != nil {
		t.Fatalf("Invalid Secret: %v", err)
	}
}

func TestRefreshExpiration(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	f := newFixture(t)

	// Create a Secret with a certificate expiring soon
	data, err := certificate.GenerateSecretData(time.Now(), time.Now().Add(5*time.Minute), generateDNSNames(cfg.GetNs(), cfg.GetSvc()))
	if err != nil {
		t.Fatalf("Failed to create the Secret: %v", err)
	}

	oldSecret := buildSecret(data)
	f.populateCache(oldSecret)

	c := f.run(stopCh)

	assert.Eventually(t, func() bool {
		newSecret, err := c.secretsLister.Secrets(cfg.GetNs()).Get(cfg.GetName())
		return err == nil && !reflect.DeepEqual(oldSecret, newSecret) && c.queue.Len() == 0
	}, waitFor, tick, "The secret hasn't been modified")

	newSecret, _ := c.secretsLister.Secrets(cfg.GetNs()).Get(cfg.GetName())

	if err := validate(newSecret); err != nil {
		t.Fatalf("Invalid Secret: %v", err)
	}
}

func TestRefreshDNSNames(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)

	f := newFixture(t)

	// Create a Secret with a dns name that doesn't match the config
	data, err := certificate.GenerateSecretData(time.Now(), time.Now().Add(365*24*time.Hour), []string{"outdated"})
	if err != nil {
		t.Fatalf("Failed to create the Secret: %v", err)
	}

	oldSecret := buildSecret(data)
	f.populateCache(oldSecret)

	c := f.run(stopCh)

	assert.Eventually(t, func() bool {
		newSecret, err := c.secretsLister.Secrets(cfg.GetNs()).Get(cfg.GetName())
		return err == nil && !reflect.DeepEqual(oldSecret, newSecret) && c.queue.Len() == 0
	}, waitFor, tick, "The secret hasn't been modified")

	newSecret, _ := c.secretsLister.Secrets(cfg.GetNs()).Get(cfg.GetName())

	if err := validate(newSecret); err != nil {
		t.Fatalf("Invalid Secret: %v", err)
	}
}

func validate(s *corev1.Secret) error {
	cert, err := certificate.GetCertFromSecret(s.Data)
	if err != nil {
		return err
	}

	expiration := certificate.GetDurationBeforeExpiration(cert)
	if expiration < 364*24*time.Hour {
		return fmt.Errorf("The certificate expires too soon: %v", expiration)
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

func (f *fixture) run(stopCh <-chan struct{}) *Controller {
	factory := informers.NewSharedInformerFactory(f.client, time.Duration(0))
	c := NewController(
		f.client,
		factory.Core().V1().Secrets(),
		func() bool { return true },
		make(chan struct{}),
		cfg,
	)

	factory.Start(stopCh)
	go c.Run(stopCh)

	return c
}

func (f *fixture) populateCache(secrets ...*corev1.Secret) {
	for _, s := range secrets {
		_, _ = f.client.CoreV1().Secrets(s.Namespace).Create(context.TODO(), s, metav1.CreateOptions{})
	}
}

func buildSecret(data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfg.GetNs(),
			Name:      cfg.GetName(),
		},
		Data: data,
	}
}

func TestDigestDNSNames(t *testing.T) {
	tests := []struct {
		name     string
		dnsNames []string
		want     uint64
	}{
		{
			name:     "nominal case",
			dnsNames: []string{"foo", "bar"},
			want:     12531106902390217800,
		},
		{
			name:     "different order, same digest",
			dnsNames: []string{"bar", "foo"},
			want:     12531106902390217800,
		},
		{
			name:     "empty dnsNames",
			dnsNames: []string{},
			want:     14695981039346656037,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := make([]string, len(tt.dnsNames))
			copy(tmp, tt.dnsNames)

			got := digestDNSNames(tt.dnsNames)
			assert.Equal(t, tt.want, got)

			// Assert we didn't mutate the input
			assert.Equal(t, tmp, tt.dnsNames)
		})
	}
}

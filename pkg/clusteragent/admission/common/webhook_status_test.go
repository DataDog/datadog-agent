// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestDigest_DeterministicAndDistinct(t *testing.T) {
	assert.Equal(t, Digest([]byte("hello")), Digest([]byte("hello")))
	assert.NotEqual(t, Digest([]byte("hello")), Digest([]byte("world")))
	assert.NotEmpty(t, Digest(nil), "digest of empty input should still be a valid (non-empty) hash")
}

func TestGetValidatingWebhookStatusV1_NotFound(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()

	_, err := GetValidatingWebhookStatusV1(context.Background(), "datadog-webhook", client)
	require.Error(t, err)
	assert.True(t, k8serrors.IsNotFound(err))
}

func TestGetValidatingWebhookStatusV1_Found(t *testing.T) {
	port := int32(443)
	path := "/mutate"
	webhook := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-webhook"},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name: "datadog.webhook.validation",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "system",
						Name:      "datadog-agent-cluster-agent-admission-controller",
						Port:      &port,
						Path:      &path,
					},
					CABundle: []byte("fake-ca-bundle"),
				},
				Rules: []admissionregistrationv1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"pods"},
						},
					},
				},
			},
		},
	}
	client := fakeclientset.NewSimpleClientset(webhook)

	status, err := GetValidatingWebhookStatusV1(context.Background(), "datadog-webhook", client)
	require.NoError(t, err)
	assert.Equal(t, "datadog-webhook", status["Name"])

	webhooks, ok := status["Webhooks"].(map[string]map[string]interface{})
	require.True(t, ok)
	entry := webhooks["datadog.webhook.validation"]
	require.NotNil(t, entry)
	assert.Contains(t, entry["Service"], "system/datadog-agent-cluster-agent-admission-controller")
	assert.Contains(t, entry["Service"], "Port: 443")
	assert.Contains(t, entry["Service"], "Path: /mutate")
	assert.Equal(t, Digest([]byte("fake-ca-bundle")), entry["CA bundle digest"])
	assert.Contains(t, entry, "Rule 1")
}

func TestGetValidatingWebhookStatusV1_ServiceDefaults(t *testing.T) {
	webhook := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-webhook"},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name: "datadog.webhook.validation",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "system",
						Name:      "svc",
					},
				},
			},
		},
	}
	client := fakeclientset.NewSimpleClientset(webhook)

	status, err := GetValidatingWebhookStatusV1(context.Background(), "datadog-webhook", client)
	require.NoError(t, err)
	webhooks := status["Webhooks"].(map[string]map[string]interface{})
	entry := webhooks["datadog.webhook.validation"]
	assert.Contains(t, entry["Service"], "Port: None (default 443)")
	assert.Contains(t, entry["Service"], "Path: None")
}

func TestGetMutatingWebhookStatusV1_NotFound(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()

	_, err := GetMutatingWebhookStatusV1(context.Background(), "datadog-webhook", client)
	require.Error(t, err)
	assert.True(t, k8serrors.IsNotFound(err))
}

func TestGetMutatingWebhookStatusV1_Found(t *testing.T) {
	webhook := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-webhook"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: "datadog.webhook.agent.config",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					CABundle: []byte("fake-ca-bundle"),
				},
			},
		},
	}
	client := fakeclientset.NewSimpleClientset(webhook)

	status, err := GetMutatingWebhookStatusV1(context.Background(), "datadog-webhook", client)
	require.NoError(t, err)
	assert.Equal(t, "datadog-webhook", status["Name"])
	webhooks := status["Webhooks"].(map[string]map[string]interface{})
	assert.Contains(t, webhooks, "datadog.webhook.agent.config")
}

func TestGetValidatingWebhookStatusV1beta1_NotFound(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()

	_, err := GetValidatingWebhookStatusV1beta1(context.Background(), "datadog-webhook", client)
	require.Error(t, err)
	assert.True(t, k8serrors.IsNotFound(err))
}

func TestGetMutatingWebhookStatusV1beta1_NotFound(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()

	_, err := GetMutatingWebhookStatusV1beta1(context.Background(), "datadog-webhook", client)
	require.Error(t, err)
	assert.True(t, k8serrors.IsNotFound(err))
}

func TestGetMutatingWebhookStatusV1beta1_Found(t *testing.T) {
	webhook := &admissionregistrationv1beta1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-webhook"},
		Webhooks: []admissionregistrationv1beta1.MutatingWebhook{
			{
				Name: "datadog.webhook.agent.config",
				ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
					CABundle: []byte("fake-ca-bundle"),
				},
			},
		},
	}
	client := fakeclientset.NewSimpleClientset(webhook)

	status, err := GetMutatingWebhookStatusV1beta1(context.Background(), "datadog-webhook", client)
	require.NoError(t, err)
	assert.Equal(t, "datadog-webhook", status["Name"])
}

func TestGetValidatingWebhookStatusV1_PropagatesNonNotFoundError(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("get", "validatingwebhookconfigurations", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewForbidden(schema.GroupResource{Resource: "validatingwebhookconfigurations"}, "datadog-webhook", errors.New("forbidden"))
	})

	_, err := GetValidatingWebhookStatusV1(context.Background(), "datadog-webhook", client)
	require.Error(t, err)
	assert.False(t, k8serrors.IsNotFound(err))
}

func TestGetMutatingWebhookStatusV1_PropagatesNonNotFoundError(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	client.PrependReactor("get", "mutatingwebhookconfigurations", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewForbidden(schema.GroupResource{Resource: "mutatingwebhookconfigurations"}, "datadog-webhook", errors.New("forbidden"))
	})

	_, err := GetMutatingWebhookStatusV1(context.Background(), "datadog-webhook", client)
	require.Error(t, err)
	assert.False(t, k8serrors.IsNotFound(err))
}

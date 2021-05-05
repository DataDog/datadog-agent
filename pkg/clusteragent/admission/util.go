// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver

package admission

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"

	admiv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// generateWebhooks returns mutating webhooks based on the configuration
func generateWebhooks() []admiv1beta1.MutatingWebhook {
	webhooks := []admiv1beta1.MutatingWebhook{}
	labelSelector := buildLabelSelector()

	// DD_AGENT_HOST injection
	if config.Datadog.GetBool("admission_controller.inject_config.enabled") {
		webhook := getWebhookSkeleton("config", config.Datadog.GetString("admission_controller.inject_config.endpoint"))
		webhook.ObjectSelector = labelSelector.DeepCopy()
		webhooks = append(webhooks, webhook)
	}

	// DD_ENV, DD_VERSION, DD_SERVICE injection
	if config.Datadog.GetBool("admission_controller.inject_tags.enabled") {
		webhook := getWebhookSkeleton("tags", config.Datadog.GetString("admission_controller.inject_tags.endpoint"))
		webhook.ObjectSelector = labelSelector.DeepCopy()
		webhooks = append(webhooks, webhook)
	}

	return webhooks
}

// buildLabelSelector returns the mutating webhooks object selector on the configuration
func buildLabelSelector() *metav1.LabelSelector {
	if config.Datadog.GetBool("admission_controller.mutate_unlabelled") {
		// Accept all, ignore pods if they're explicitly filtered-out
		return &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      EnabledLabelKey,
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{"false"},
				},
			},
		}
	}

	// Ignore all, accept pods if they're explicitly allowed
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			EnabledLabelKey: "true",
		},
	}
}

func getWebhookSkeleton(nameSuffix, path string) admiv1beta1.MutatingWebhook {
	failurePolicy := admiv1beta1.Ignore
	sideEffects := admiv1beta1.SideEffectClassNone
	servicePort := int32(443)
	timeout := config.Datadog.GetInt32("admission_controller.timeout_seconds")
	return admiv1beta1.MutatingWebhook{
		Name: strings.ReplaceAll(fmt.Sprintf("%s.%s", config.Datadog.GetString("admission_controller.webhook_name"), nameSuffix), "-", "."),
		ClientConfig: admiv1beta1.WebhookClientConfig{
			Service: &admiv1beta1.ServiceReference{
				Namespace: common.GetResourcesNamespace(),
				Name:      config.Datadog.GetString("admission_controller.service_name"),
				Port:      &servicePort,
				Path:      &path,
			},
		},
		Rules: []admiv1beta1.RuleWithOperations{
			{
				Operations: []admiv1beta1.OperationType{
					admiv1beta1.Create,
				},
				Rule: admiv1beta1.Rule{
					APIGroups:   []string{""},
					APIVersions: []string{"v1"},
					Resources:   []string{"pods"},
				},
			},
		},
		FailurePolicy:  &failurePolicy,
		SideEffects:    &sideEffects,
		TimeoutSeconds: &timeout,
	}
}

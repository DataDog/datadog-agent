// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package types contains the share webhook types such as Interfaces and Enums
package types

import (
	"context"

	admiv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
)

// Webhook represents an admission webhook
type Webhook interface {
	// Name returns the name of the webhook
	Name() string
	// WebhookType Type returns the type of the webhook
	WebhookType() common.WebhookType
	// IsEnabled returns whether the webhook is enabled
	IsEnabled() bool
	// Start starts the webhook
	Start(context.Context) error
	// Endpoint returns the endpoint of the webhook
	Endpoint() string
	// Resources returns the kubernetes resources for which the webhook should
	// be invoked.
	// The key is the API group, and the value is a list of resources.
	Resources() ResourceRuleConfigList
	// Operations returns the operations on the resources specified for which
	// the webhook should be invoked
	Operations() []admiv1.OperationType
	// LabelSelectors returns the label selectors that specify when the webhook
	// should be invoked
	LabelSelectors(useNamespaceSelector bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector)
	// MatchConditions returns the Match Conditions used for fine-grained
	// request filtering
	MatchConditions() []admiv1.MatchCondition
	// WebhookFunc runs the logic of the webhook and returns the admission response
	WebhookFunc() admission.WebhookFunc
}

// ResourceRuleConfig represents a rule configuration for a resource
type ResourceRuleConfig struct {
	APIGroups      []string
	APIVersions    []string
	Operations     []string
	Resources      []string
	NamespaceScope bool
}

// ResourceRuleConfigList is a list of ResourceRuleConfig
type ResourceRuleConfigList []ResourceRuleConfig

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package handlers provides product-specific handlers for the Datadog Instrumentation CRD controller.
package handlers

import (
	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
)

// Deps contains dependencies used to construct DatadogInstrumentation product handlers.
// Product-specific services that are shared with other integration surfaces, such as
// admission webhooks, should be added here rather than constructed in the generic
// controller startup path.
type Deps struct {
	// IsLeader should be used if the handler should only perform actions when the cluster agent is the leader.
	IsLeader func() bool

	// CheckStore is used as a shared store for check configurations between the AD handler and cluster agent API.
	CheckStore *CheckStore

	// ServiceCheckTemplateStore holds check templates for Service-targeted DDI CRs.
	// Shared with the endpoint slices CR config provider that resolves templates into endpoint configs.
	ServiceCheckTemplateStore *ServiceCheckTemplateStore
}

// DefaultHandlers returns the product handlers registered for the shared controller.
func DefaultHandlers(deps *Deps) []instrumentation.Handler {
	return []instrumentation.Handler{
		NewChecksHandler(deps),
		NewLogsHandler(deps),
	}
}

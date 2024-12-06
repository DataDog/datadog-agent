// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package admission contains the admission controller logic, as well as the webhook implementations.
//
// Admission webhooks can intercept requests to the Kubernetes API server and
// either validate or mutate the Kubernetes request before the operation specified
// in the request is applied.
// To learn more about admission webhooks, see the official Kubernetes documentation:
// https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers
//
// In general, each webhook should be implemented in its own Go
// package.
// Dependencies between webhooks should be avoided. If there's a dependency
// between webhooks, consider grouping them in the same webhook instead.
//
// Try to avoid depending on the order in which webhooks are executed.
// When this cannot be avoided, keep in mind that the order in which the
// webhooks are executed is the order in which they are returned by the
// "generateWebhooks" function in the "webhook" package.
//
// Each webhook needs to implement the "Webhook" interface.
// Here's a brief description of each function along with its purpose:
// - Name: This is the name of the webhook, used to identify it. The name
// appears in some telemetry tags.
// - WebhookType: Indicates the type of webhook, with possible values being "mutating" or
// "validating".
// - IsEnabled: Indicates whether the webhook is enabled. In general, the
// recommendation is to disable the webhook by default, unless it's needed by a
// core feature that should be enabled for everyone that deploys the Datadog
// Agent on Kubernetes.
// - Endpoint: The endpoint where the webhook is registered.
// - Resources: The Kubernetes resources that the webhook is interested in. For
// example, pods, deployments, etc.
// - Operations: The operations applied to the resources that the webhook is
// interested in. For example: create, update, delete, etc.
// - LabelSelectors: Enables you to filter the requests that the webhook receives.
// For example, you can configure the webhook to only receive requests about pods
// that have a specific label. For performance reasons, try to
// minimize the number of requests that the webhook receives. The label
// selectors help with that. There are some default label selectors defined
// in the "common" package.
// - MatchConditions: Enables you to filter the requests received by the webhook
// with a more fine-grained approach than label selectors, using the CEL language.
// - WebhookFunc: The function that runs the logic of the webhook and returns the admission response.
//
// As with any other feature, webhooks can be configured using the Datadog
// configuration. When adding new configuration parameters, try to follow
// the convention of the other webhooks. The configuration parameters
// for a webhook should be under the "admission_controller.name_of_the_webhook"
// key.
//
// Webhooks emit telemetry metrics. Each webhook can define its own
// metrics as needed, but some metrics like "webhooks_received" are common
// to all webhooks and defined in common code, so new webhooks can use
// them without having to define them again.
//
// When implementing a new webhook, keep performance in mind. For example, if
// the webhook reacts upon the creation of a new pod, it could slow down the pod
// creation process.
package admission

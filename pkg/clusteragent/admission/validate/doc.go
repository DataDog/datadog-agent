// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package validate contains validating webhooks registered in the admission
// controller.
//
// The general idea of validating webhooks is to intercept requests to the
// Kubernetes API server and either admit or refuse the Kubernetes request before the operation
// specified in the request is applied. For example, a validating webhook can be
// configured to receive all the requests about creating or updating a pod, and refuse pod creation
// if they don't respect specific rules. Validating webhooks can be used to
// enforce security policies, best practices, etc.
// To learn more about validating webhooks, see the official Kubernetes documentation:
// https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/
//
// In general, each validating webhook should be implemented in its own Go
// package. If there are some related webhooks that share some code, they can be
// grouped in the same package. For example, the CWS webhooks are all in the
// same package.
//
// Each validating webhook needs to implement the "Webhook" interface.
// Here's a brief description of each function and what they are used for:
// - Name: it's the name of the webhook. It's used to identify it. The name
// appears in some telemetry tags.
// - WebhookType: it's the type of the webhook. It can be either "mutating" or
// "validating".
// - IsEnabled: returns whether the webhook is enabled or not. In general, the
// recommendation is to disable the webhook by default unless it's needed by a
// core feature that should be enabled for everyone that deploys the Datadog
// Agent on Kubernetes.
// - Endpoint: the endpoint where the webhook is registered.
// - Resources: the Kubernetes resources that the webhook is interested in. For
// example, pods, deployments, etc.
// - Operations: the operations applied to the resources that the webhook is
// interested in. For example: create, update, delete, etc.
// - LabelSelectors: allow us to filter the requests that the webhook receives.
// For example, we can configure the webhook to only receive requests about pods
// that have a specific label. For performance reasons, we should try to
// minimize the number of requests that the webhook receives. The label
// selectors help us with that. There are some default label selectors defined
// in the "common" package.
// - WebhookFunc: the function that runs the logic of the webhook and returns the admission response.
//
// As any other feature, validating webhooks can be configured using the Datadog
// configuration. When adding new configuration parameters, please try to follow
// the convention of the other validating webhooks. The configuration parameters
// for a webhook should be under the "admission_controller.name_of_the_webhook"
// key.
//
// Dependencies between webhooks should be avoided. If there's a dependency
// between webhooks, consider grouping them in the same webhook instead.
//
// We should try to avoid depending on the order in which webhooks are executed.
// When this cannot be avoided, keep in mind that the order in which the
// webhooks are executed is the order in which they are returned by the
// "generateWebhooks" function in the "webhook" package.
//
// Validating webhooks emit telemetry metrics. Each webhook can define its own
// metrics as needed but some metrics like "validation_attempts" or
// "webhooks_received" are common to all webhooks and defined in common code, so
// new webhooks can use them without having to define them again.
//
// When implementing a new webhook keep performance in mind. For instance, if
// the webhook reacts upon the creation of a new pod, it could slow down the pod
// creation process.
package validate

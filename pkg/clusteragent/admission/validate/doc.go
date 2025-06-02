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
// https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers
package validate

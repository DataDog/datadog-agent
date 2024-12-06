// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package mutate contains mutating webhooks registered in the admission
// controller.
//
// The general idea of mutating webhooks is to intercept requests to the
// Kubernetes API server and modify the Kubernetes objects before the operation
// specified in the request is applied. For example, a mutating webhook can be
// configured to receive all the requests about creating or updating a pod, and
// modify the pod to enforce some defaults. A typical example is to intercept
// requests to create a pod and add some environment variables or volumes to the
// pod to enable some functionality automatically. This saves the user from
// having to add environment variables or volumes manually on each pod in their
// cluster.
// To learn more about mutating webhooks, see the official Kubernetes documentation:
// https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers
package mutate

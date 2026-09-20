// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package tagruleswebhook implements the validating admission webhook for
// TagRule CRs (agent.datadoghq.com/v1alpha1).
//
// It is deliberately a separate HTTPS server from the Cluster Agent's
// admission controller: it serves one endpoint, POST /validate/tagrules,
// answers AdmissionReviews, and keeps its validation logic in one readable
// file. The webhook enforces what the TagRule CRD's schema cannot:
//
//   - structural invariants mirrored from the CRD's x-kubernetes-validations
//     and the controller's own validateRule (fail-closed against rules that
//     bypassed validation),
//   - CEL compile checks for every expression the controller evaluates
//     (value.bool.expression, value.stringSet.expression, source.name,
//     source.namespace) with the same environment the controller uses,
//   - no duplicate values in string sets,
//   - global tag-key uniqueness across all TagRule CRs in the cluster,
//   - spec.tag immutability across updates.
//
// Any parse or handler error is denied (fail-closed); the
// ValidatingWebhookConfiguration should also use failurePolicy: Fail.
package tagruleswebhook

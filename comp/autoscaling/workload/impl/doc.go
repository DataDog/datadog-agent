// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package impl implements the DatadogPodAutoscaler controller and
// related components to implement horizontal and vertical pod autoscaling.
package impl

// The main controller can perform both horizontal and vertical scaling with the logic in
// the horizontal and vertical controllers.
//
// When a PodAutoscalerInternal object is fed to the verticalController, it verifies the autoscaler
// configuration, extracts the relevant target deployment based on the specifications,
// and checks for vertical scaling recommendations. If any discrepancies between the
// current resources allocated to the deployment and the recommended values exist, and
// no rollout is currently ongoing, the verticalController adds an annotation
// `autoscaling.datadoghq.com/scaling-hash` to the pod spec template then requeues the
// object to make sure the rollout completed.
//
// For deployments, we identify ongoing rollouts by checking if all the pods are owned
// by the same ReplicaSet.
//
// Importantly, the verticalController only acts on types of Kubernetes resources explicitly
// supported in its context (currently only deployments).

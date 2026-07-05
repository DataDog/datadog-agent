// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package cluster contains the controller for cluster autoscaling.
package cluster

// EventSourceComponent is the Kubernetes event source component name for the cluster autoscaler,
// shared by all sub-components (cluster autoscaling, spot scheduling).
const EventSourceComponent = "datadog-cluster-autoscaler"

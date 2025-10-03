// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package ecsmanagedinstance implements the ECS Managed Instance metrics provider.
// The ECS Managed Instance metrics provider collects container metrics from ECS tasks
// running on managed instances using the ECS metadata API. This is used when running
// the Datadog Agent as a sidecar on ECS Managed Instances.
package ecsmanagedinstance

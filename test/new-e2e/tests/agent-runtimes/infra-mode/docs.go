// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inframode provides e2e tests for infrastructure mode functionality (basic,
// end_user_device, cloud_cost_only)
//
// These suites cover what a host emits: the infra_mode marker on the host-tags
// payload, and which checks' metrics carry it. The marker on the resource
// payloads needs workloads to report on: orchestrator resources and manifests
// need a cluster and are covered by TestKindInfraModeSuite in
// test/new-e2e/tests/orchestrator, containers need a container runtime and are
// covered by TestInfraModeSuite in test/new-e2e/tests/process.
package inframode

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package policies is a self-contained, dependency-free policy model and
// tri-state evaluator for Single Step Instrumentation (SSI) workload targeting.
//
// It mirrors the semantics of the dd-policy-engine C evaluator (TRUE / FALSE /
// ABSTAIN over an AND/OR/NOT tree of leaf evaluators) in pure Go, so the
// cluster-agent can evaluate SSI policies natively without CGO. The package has
// no dependency on the surrounding autoinstrumentation package, which keeps it
// trivial to extract into dd-policy-engine/go later.
//
// It also provides a converter (FromTargets) that lowers the existing
// Kubernetes "targets" configuration into the policy model, giving functional
// parity with the native target matcher while opening the door to richer,
// remote-config-distributed policies.
package policies

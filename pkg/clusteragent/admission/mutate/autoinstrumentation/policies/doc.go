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
// Policies are produced either by parsing the remote-config dd-wls document
// (ParsePolicies) or, for the static agent configuration, by lowering the
// "targets" configuration into the policy model. The latter lives in the
// autoinstrumentation package and builds rule trees with the exported node
// constructors (And, Or, Not, Leaf, ...) so this package stays free of any
// knowledge of the targets configuration shape.
package policies

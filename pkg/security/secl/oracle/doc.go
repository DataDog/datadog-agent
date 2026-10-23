// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package oracle holds a golden corpus recording what the SECL pipeline
// (parsing, compilation, evaluation, partial evaluation, field values,
// approvers, discarders, macros and variables) observably does, so that the
// pipeline can be rewritten while proving it still behaves the same.
//
// Every case of testdata/corpus/*.yaml is loaded as a policy into a RuleSet
// built on the real model, then a sequence of events is replayed against it.
// The resulting observations are compared against testdata/golden/*.yaml.
// Regenerate the golden files with:
//
//	go test -tags test ./oracle -update
//
// The corpus was grown under coverage and mutation testing: every closure,
// approver, discarder and variable result and every `if` of the pipeline,
// broken one at a time, fails at least one case unless it is unreachable.
package oracle

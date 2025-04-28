// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !unix

package compliance

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EvaluateRegoRule evaluates the given rule and resolved inputs map against
// the rule's rego program.
func EvaluateRegoRule(_ context.Context, _ ResolvedInputs, _ *Benchmark, _ *Rule) []*CheckEvent {
	log.Errorf("rego evaluator is not supported on this platform")
	return nil
}

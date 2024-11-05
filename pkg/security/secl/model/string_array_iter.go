// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import "github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"

func newAncestorsIterator[T any](iter *ProcessAncestorsIterator, ctx *eval.Context, ev *Event, perIter func(ev *Event, pce *ProcessCacheEntry) T) []T {
	var results []T

	for pce := iter.Front(ctx); pce != nil; pce = iter.Next() {
		if !pce.ProcessContext.Process.IsNotKworker() {
			var defaultValue T
			results = append(results, defaultValue)
		} else {
			results = append(results, perIter(ev, pce))
		}
	}

	return results
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import "github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"

func newAncestorsIterator[T any](iter *ProcessAncestorsIterator, ctx *eval.Context, ev *Event, perIter func(ev *Event, pce *ProcessCacheEntry) T) []T {
	results := make([]T, 0, ctx.CachedAncestorsCount)
	for pce := iter.Front(ctx); pce != nil; pce = iter.Next() {
		results = append(results, perIter(ev, pce))
	}
	ctx.CachedAncestorsCount = len(results)

	return results
}

func newAncestorsIteratorArray[T any](iter *ProcessAncestorsIterator, ctx *eval.Context, ev *Event, perIter func(ev *Event, pce *ProcessCacheEntry) []T) []T {
	results := make([]T, 0, ctx.CachedAncestorsCount)
	ancestorsCount := 0
	for pce := iter.Front(ctx); pce != nil; pce = iter.Next() {
		results = append(results, perIter(ev, pce)...)
		ancestorsCount++
	}
	ctx.CachedAncestorsCount = ancestorsCount

	return results
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import "github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"

// AncestorsIterator is a generic interface that iterators must implement
type AncestorsIterator[T any] interface {
	Front(ctx *eval.Context) T
	Next(ctx *eval.Context) T
	At(ctx *eval.Context, regID eval.RegisterID, pos int) T
	Len(ctx *eval.Context) int
}

// Helper function to check if a value is nil
func isNil[V comparable](v V) bool {
	var zero V
	return v == zero
}

func newAncestorsIterator[T any, V comparable](iter AncestorsIterator[V], field eval.Field, ctx *eval.Context, ev *Event, perIter func(ev *Event, current V) T) []T {
	results := make([]T, 0, ctx.AncestorsCounters[field])
	for entry := iter.Front(ctx); !isNil(entry); entry = iter.Next(ctx) {
		results = append(results, perIter(ev, entry))
	}
	ctx.AncestorsCounters[field] = len(results)

	return results
}

func newAncestorsIteratorArray[T any, V comparable](iter AncestorsIterator[V], field eval.Field, ctx *eval.Context, ev *Event, perIter func(ev *Event, current V) []T) []T {
	results := make([]T, 0, ctx.AncestorsCounters[field])
	ancestorsCount := 0
	for entry := iter.Front(ctx); !isNil(entry); entry = iter.Next(ctx) {
		results = append(results, perIter(ev, entry)...)
		ancestorsCount++
	}
	ctx.AncestorsCounters[field] = ancestorsCount

	return results
}

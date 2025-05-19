// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// Iterator is a generic interface that iterators must implement
type Iterator[T any] interface {
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

func newIterator[T any, V comparable](iter Iterator[V], field eval.Field, ctx *eval.Context, ev *Event, perIterCb func(ev *Event, current V) T) []T {
	results := make([]T, 0, ctx.IteratorCountCache[field])
	for entry := iter.Front(ctx); !isNil(entry); entry = iter.Next(ctx) {
		results = append(results, perIterCb(ev, entry))
	}
	ctx.IteratorCountCache[field] = len(results)

	return results
}

func newIteratorArray[T any, V comparable](iter Iterator[V], field eval.Field, ctx *eval.Context, ev *Event, perIterCb func(ev *Event, current V) []T) []T {
	results := make([]T, 0, ctx.IteratorCountCache[field])
	count := 0
	for entry := iter.Front(ctx); !isNil(entry); entry = iter.Next(ctx) {
		results = append(results, perIterCb(ev, entry)...)
		count++
	}
	ctx.IteratorCountCache[field] = count

	return results
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hook

// noopHook is a Hook implementation that discards all published payloads and ignores subscriptions.
type noopHook[T any] struct{}

func (noopHook[T]) Name() string                                       { return "noop" }
func (noopHook[T]) Publish(_ string, _ T)                              {}
func (noopHook[T]) Subscribe(_ string, _ func(T)) (unsubscribe func()) { return func() {} }

// NewNoopHook returns a Hook that silently discards all published payloads.
// Use this in tests or in code paths where hook observation is optional.
func NewNoopHook[T any]() Hook[T] { return noopHook[T]{} }

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package types

import "context"

// IntermediateResultPublisher allows actions to publish intermediate results during execution.
type IntermediateResultPublisher interface {
	Publish(ctx context.Context, result string, sequenceNumber int64) error
}

type intermediateResultPublisherKey struct{}

// ContextWithPublisher returns a new context with the publisher attached.
func ContextWithPublisher(ctx context.Context, publisher IntermediateResultPublisher) context.Context {
	return context.WithValue(ctx, intermediateResultPublisherKey{}, publisher)
}

// PublisherFromContext extracts the publisher from the context, if present.
func PublisherFromContext(ctx context.Context) (IntermediateResultPublisher, bool) {
	publisher, ok := ctx.Value(intermediateResultPublisherKey{}).(IntermediateResultPublisher)
	return publisher, ok
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import "context"

type installProgressContextKey struct{}

// WithInstallProgress attaches a progress setter function to the context.
// The daemon uses this to receive completion updates (0.0–1.0) from inside
// an ongoing install operation without introducing a circular dependency.
func WithInstallProgress(ctx context.Context, setter func(float32)) context.Context {
	return context.WithValue(ctx, installProgressContextKey{}, setter)
}

// setInstallProgress invokes the progress setter stored in ctx, if any.
func setInstallProgress(ctx context.Context, v float32) {
	if setter, ok := ctx.Value(installProgressContextKey{}).(func(float32)); ok {
		setter(v)
	}
}

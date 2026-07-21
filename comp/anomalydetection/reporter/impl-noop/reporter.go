// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package noopimpl provides a no-op reporter implementation.
// Reporters in production register via the anomalydetection_reporters Fx group.
// This stub satisfies the component structure linter; the real implementation
// lives in reporter/impl.
package noopimpl

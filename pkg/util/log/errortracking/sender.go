// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package errortracking

import (
	"context"
	"log/slog"
)

// Sender ships a batch of records to a remote ingest. Implementations live
// outside this package (typically the COAT component at
// comp/core/agenttelemetry/, via its SendErrorLogs method) so that the
// foundational pkg/util/log subtree stays free of HTTP and configuration
// dependencies.
//
// Implementations MUST be safe for concurrent use, although the Pipeline
// only invokes Send from a single goroutine.
//
// A non-nil error signals a retryable transport failure (network error,
// 5xx status). The Pipeline will retry the same batch once and then drop
// it; this prevents a misbehaving backend from back-pressuring the source.
// Terminal failures (4xx, malformed payload) should be logged internally
// and reported as a nil error so the Pipeline does not waste a retry on
// something that will not succeed.
type Sender interface {
	Send(ctx context.Context, batch []slog.Record) error
}

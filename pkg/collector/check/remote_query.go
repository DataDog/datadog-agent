// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package check

import (
	"context"
	"errors"
)

// ErrRemoteQueryCancelled means unscheduling closed the check to remote calls.
var ErrRemoteQueryCancelled = errors.New("check is being unscheduled")

// RemoteQuerier is an optional boundary implemented by checks supporting remote
// queries. Calls may overlap each other and scheduled Run calls. Only JSON
// requests and final outcome JSON cross this boundary, never result data.
// Implementations must not start work if ctx has ended. The Python bridge checks
// this while holding the GIL before entering Python. Cancellation is idempotent;
// the core may repeat it to cover cancellation racing the run's registration.
type RemoteQuerier interface {
	SupportsRemoteQueries() bool
	RemoteQuery(ctx context.Context, request string) (string, error)
	CancelRemoteQuery(runID string) error
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"errors"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// statusError is an intake response the destination gave up on or will retry. It
// unwraps to errClient or errServer, so existing errors.Is checks keep working,
// and its message is unchanged ("client error: 403 Forbidden"). It exists so a
// drop can say which status the intake answered.
type statusError struct {
	class  error
	code   int
	status string
}

func newStatusError(class error, resp *http.Response) *statusError {
	return &statusError{class: class, code: resp.StatusCode, status: resp.Status}
}

func (e *statusError) Error() string {
	return e.class.Error() + ": " + e.status
}

func (e *statusError) Unwrap() error {
	return e.class
}

// dropReasonFor maps the error a payload was dropped on to the health issue's
// closed set of reasons.
func dropReasonFor(err error) metrics.DropReason {
	var status *statusError
	if errors.As(err, &status) {
		switch status.code {
		case http.StatusUnauthorized, http.StatusForbidden:
			return metrics.DropReasonAuthRejected
		case http.StatusRequestEntityTooLarge:
			return metrics.DropReasonPayloadTooLarge
		case http.StatusBadRequest:
			return metrics.DropReasonBadRequest
		}
	}
	if errors.Is(err, errCredentialNotReady) {
		return metrics.DropReasonCredentialUnavailable
	}
	return metrics.DropReasonSendFailed
}

// dropTargetFor names a destination in the dropped-logs health issue. The host,
// never the URL, so no path or query string ends up in the issue.
func dropTargetFor(endpoint config.Endpoint, destMeta *client.DestinationMetadata, shouldRetry bool) metrics.DropTarget {
	// Senders build reliable destinations in GetReliableEndpoints order, which
	// starts with the main endpoint, so reliable endpoint 0 is the primary. A noop
	// metadata carries no position, so fall back on whether the destination retries.
	var kind metrics.DropTargetKind
	switch {
	case endpoint.IsMRF:
		kind = metrics.DropTargetMRF
	case !destMeta.ReportingEnabled && shouldRetry:
		kind = metrics.DropTargetPrimary
	case !destMeta.ReportingEnabled, destMeta.Kind() == "unreliable":
		kind = metrics.DropTargetBestEffort
	case destMeta.EndpointID() == "0":
		kind = metrics.DropTargetPrimary
	default:
		kind = metrics.DropTargetAdditional
	}
	return metrics.DropTarget{Host: endpoint.Host, Kind: kind}
}

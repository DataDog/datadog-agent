// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
)

func convertExternalCallError(err error, query string, retryAfter time.Time) error {
	if err == nil {
		return newQueryError(query, "unknown error", retryAfter)
	}

	apiErr := &autoscalers.APIError{}
	switch {
	case errors.As(err, &apiErr):
		return newBatchError(err, retryAfter)
	default:
		// Should only from autoscalers.ProcessingError, but just in case setting as default
		return newQueryError(query, err.Error(), retryAfter)
	}
}

// queryError represents an error at the query level (not all queries failed).
type queryError struct {
	query      string
	reason     string
	retryAfter time.Time
}

func newOutdatedQueryError(query string) *queryError {
	return newQueryError(query, "outdated result, check MaxAge setting", time.Time{})
}

func newMissingResultQueryError(query string) *queryError {
	return newQueryError(query, "missing result from reply", time.Time{})
}

func newQueryError(query, reason string, retryAfter time.Time) *queryError {
	return &queryError{
		query:      query,
		reason:     reason,
		retryAfter: retryAfter,
	}
}

func (e *queryError) Error() string {
	if !e.retryAfter.IsZero() {
		return fmt.Sprintf("Processing data from API failed, reason: %s, query was: %s, will retry at: %s", e.reason, e.query, e.retryAfter.Format(time.DateTime))
	}
	return fmt.Sprintf("Processing data from API failed, reason: %s, query was: %s", e.reason, e.query)
}

func (e *queryError) Is(err error) bool {
	queryErr, ok := err.(*queryError)
	if !ok {
		return false
	}

	// We skip comparison of exact time, it's enough to be both sets or both zero.
	return queryErr.query == e.query &&
		queryErr.reason == e.reason &&
		((queryErr.retryAfter.IsZero() && e.retryAfter.IsZero()) || (!queryErr.retryAfter.IsZero() && !e.retryAfter.IsZero()))
}

// batchError represents a global backend error for all queries.
type batchError struct {
	err        error
	retryAfter time.Time
}

func newBatchError(err error, retryAfter time.Time) *batchError {
	return &batchError{
		err:        err,
		retryAfter: retryAfter,
	}
}

func (e *batchError) Error() string {
	if !e.retryAfter.IsZero() {
		return fmt.Sprintf("Datadog API call error (all queries), error: %s, will retry at: %s", e.err, e.retryAfter.Format(time.DateTime))
	}
	return fmt.Sprintf("Datadog API call error (all queries), error: %s", e.err)
}

func (e *batchError) Unwrap() error {
	return e.err
}

func (e *batchError) Is(err error) bool {
	batchErr, ok := err.(*batchError)
	if !ok {
		return false
	}

	// We skip comparison of exact time, it's enough to be both sets or both zero.
	return e.err.Error() == batchErr.err.Error() &&
		((batchErr.retryAfter.IsZero() && e.retryAfter.IsZero()) || (!batchErr.retryAfter.IsZero() && !e.retryAfter.IsZero()))
}

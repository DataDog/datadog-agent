// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"fmt"
)

// InvalidMetricError represents a generic invalid metric error.
type InvalidMetricError struct {
	Err   error
	Query string
}

func (e *InvalidMetricError) Error() string {
	return fmt.Sprintf("Invalid metric error: %v, query was: %s", e.Err, e.Query)
}

// NewInvalidMetricError initializes an InvalidMetricError.
func NewInvalidMetricError(err error, query string) *InvalidMetricError {
	return &InvalidMetricError{
		Err:   err,
		Query: query,
	}
}

// InvalidMetricErrorWithRetries represents an invalid metric error with retry information.
type InvalidMetricErrorWithRetries struct {
	Err        error
	Query      string
	RetryAfter string
}

func (e *InvalidMetricErrorWithRetries) Error() string {
	return fmt.Sprintf("Invalid metric error: %v, query was: %s, will retry after %s", e.Err, e.Query, e.RetryAfter)
}

// NewInvalidMetricErrorWithRetries initializes an InvalidMetricErrorWithRetries.
func NewInvalidMetricErrorWithRetries(err error, query string, retryAfter string) *InvalidMetricErrorWithRetries {
	return &InvalidMetricErrorWithRetries{
		Err:        err,
		Query:      query,
		RetryAfter: retryAfter,
	}
}

// InvalidMetricOutdatedError represents an error for outdated metric results.
type InvalidMetricOutdatedError struct {
	Query string
}

func (e *InvalidMetricOutdatedError) Error() string {
	return fmt.Sprintf("Query returned outdated result, check MaxAge setting, query: %s", e.Query)
}

// NewInvalidMetricOutdatedError initializes an InvalidMetricOutdatedError.
func NewInvalidMetricOutdatedError(query string) *InvalidMetricOutdatedError {
	return &InvalidMetricOutdatedError{
		Query: query,
	}
}

// InvalidMetricNotFoundError represents an error when the metric data is not found in the query result.
type InvalidMetricNotFoundError struct {
	Query string
}

func (e *InvalidMetricNotFoundError) Error() string {
	return fmt.Sprintf("Unexpected error, query data not found in result, query: %s", e.Query)
}

// NewInvalidMetricNotFoundError initializes an InvalidMetricNotFoundError.
func NewInvalidMetricNotFoundError(query string) *InvalidMetricNotFoundError {
	return &InvalidMetricNotFoundError{
		Query: query,
	}
}

// InvalidMetricGlobalError represents a global backend error for all queries.
type InvalidMetricGlobalError struct{}

func (e *InvalidMetricGlobalError) Error() string {
	return "Global error (all queries) from backend, invalid syntax in query? Check Cluster Agent leader logs for details"
}

// NewInvalidMetricGlobalError initializes an InvalidMetricGlobalError.
func NewInvalidMetricGlobalError() *InvalidMetricGlobalError {
	return &InvalidMetricGlobalError{}
}

// InvalidMetricGlobalErrorWithRetries represents a global backend error for all queries with retry information.
type InvalidMetricGlobalErrorWithRetries struct {
	BatchSize  int
	RetryAfter string
}

func (e *InvalidMetricGlobalErrorWithRetries) Error() string {
	return fmt.Sprintf("Global error (all queries, batch size %d) from backend, invalid syntax in query? Check Cluster Agent leader logs for details. Will retry after %s", e.BatchSize, e.RetryAfter)
}

// NewInvalidMetricGlobalErrorWithRetries initializes an InvalidMetricGlobalErrorWithRetries.
func NewInvalidMetricGlobalErrorWithRetries(batchSize int, retryAfter string) *InvalidMetricGlobalErrorWithRetries {
	return &InvalidMetricGlobalErrorWithRetries{
		BatchSize:  batchSize,
		RetryAfter: retryAfter,
	}
}

// RateLimitError represents a rate limit error.
type RateLimitError struct {
	Err error
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("Global error: Datadog API rate limit exceeded")
}

// NewRateLimitError initializes a RateLimitError.
func NewRateLimitError() *RateLimitError {
	return &RateLimitError{}
}

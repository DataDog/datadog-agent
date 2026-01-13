// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

import (
	"errors"
	"regexp"
	"strings"
)

// APIErrorCode represents some error codes returned by the Datadog API
type APIErrorCode int

const (
	// UnknownAPIError is a generic error code (unchecked or unknown error)
	UnknownAPIError APIErrorCode = 0
	// DatadogAPIError is the error code returned by the Datadog API (HTTP 200 with "Error" JSON field)
	DatadogAPIError APIErrorCode = 1
	// OtherHTTPStatusCodeAPIError is aggregating all other HTTP status codes
	OtherHTTPStatusCodeAPIError APIErrorCode = 2
	// RateLimitExceededAPIError is the error code returned by the Datadog API when the rate limit is exceeded
	RateLimitExceededAPIError APIErrorCode = 429
	// UnprocessableEntityAPIError is the error code returned by the Datadog API when the request is malformed
	UnprocessableEntityAPIError APIErrorCode = 422

	// Dealing with lack of typed error from zorkian/go-datadog-api
	// From https://github.com/zorkian/go-datadog-api/blob/437d51d487bfc328fcedadef799fb92128bb2278/request.go#L174C4-L174C60
	// Format is fmt.Errorf("API returned error: %s", common.Error)
	zorkianDatadogErrorMessagePrefix = "API returned error: "
)

var (
	// RateLimitExceededError is the error returned when the rate limit is exceeded
	RateLimitExceededError = &APIError{Code: RateLimitExceededAPIError}
	// UnprocessableEntityError is the error returned when the request cannot be processed
	UnprocessableEntityError = &APIError{Code: UnprocessableEntityAPIError}

	// Dealing with lack of typed error from zorkian/go-datadog-api
	// From https://github.com/zorkian/go-datadog-api/blob/437d51d487bfc328fcedadef799fb92128bb2278/request.go#L142C14-L142C20
	// Format is fmt.Errorf("API error %s: %s", resp.Status, body)
	zorkianHTTPErrorRegexp = regexp.MustCompile(`^API error ([0-9]{3} ['\w ]+):`)
)

// APIError represents a global error returned by the Datadog API (full batch)
type APIError struct {
	Code APIErrorCode
	Err  error
}

// NewAPIError creates a new APIError from an error
func NewAPIError(err error) *APIError {
	errorCode := UnknownAPIError

	// Try to process untyped errors from zorkian/go-datadog-api
	errString := err.Error()
	if after, ok := strings.CutPrefix(errString, zorkianDatadogErrorMessagePrefix); ok {
		err = errors.New(strings.ReplaceAll(after, "\n", " "))
		errorCode = DatadogAPIError
	} else if matchesIdx := zorkianHTTPErrorRegexp.FindStringSubmatchIndex(errString); len(matchesIdx) == 4 {
		httpStatus := errString[matchesIdx[2]:matchesIdx[3]]
		switch httpStatus {
		case "429 Too Many Requests":
			errorCode = RateLimitExceededAPIError
		case "422 Unprocessable Entity":
			errorCode = UnprocessableEntityAPIError
		default:
			errorCode = OtherHTTPStatusCodeAPIError
		}

		err = errors.New(httpStatus)
	}

	return &APIError{
		Code: errorCode,
		Err:  err,
	}
}

// Error returns the error message
func (e *APIError) Error() string {
	return e.Err.Error()
}

// Is checks if the error is the same
func (e *APIError) Is(err error) bool {
	apiErr, ok := err.(*APIError)
	if !ok {
		return false
	}

	return apiErr.Code == e.Code
}

// Unwrap returns the wrapped error
func (e *APIError) Unwrap() error {
	return e.Err
}

// ProcessingError represents an error during the processing of API data (single query)
type ProcessingError struct {
	Err string
}

// NewProcessingError creates a new ProcessingError
func NewProcessingError(err string) *ProcessingError {
	return &ProcessingError{
		Err: err,
	}
}

// Error returns the error message
func (e *ProcessingError) Error() string {
	return e.Err
}

// Is checks if the error is the same
func (e *ProcessingError) Is(err error) bool {
	processingErr, ok := err.(*ProcessingError)
	if !ok {
		return false
	}

	return processingErr.Err == e.Err
}

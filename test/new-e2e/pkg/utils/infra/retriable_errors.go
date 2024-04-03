// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infra implements utilities to interact with a Pulumi infrastructure
package infra

type retryType string

const (
	reUp     retryType = "ReUp"     // Retry the up operation
	reCreate retryType = "ReCreate" // Retry the up operation after destroying the stack
	noRetry  retryType = "NoRetry"
)

type retriableError struct {
	errorMessage string
	retryType    retryType
}

func getKnownRetriableErrors() []retriableError {
	// Add here errors that are known to be flakes and that should be retried
	return []retriableError{
		{
			errorMessage: "i/o timeout",
			retryType:    reCreate,
		},
		{
			errorMessage: "creating EC2 Instance: IdempotentParameterMismatch:",
			retryType:    reUp,
		},
		{
			errorMessage: "InvalidInstanceID.NotFound",
			retryType:    reUp,
		},
		{
			errorMessage: "create: timeout while waiting for state to become 'tfSTABLE'",
			retryType:    reUp,
		},
		{
			// https://datadoghq.atlassian.net/browse/ADXT-1
			errorMessage: "failed attempts: dial tcp :22: connect: connection refused",
			retryType:    reCreate,
		},
	}
}

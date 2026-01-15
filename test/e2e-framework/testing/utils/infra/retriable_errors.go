// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infra implements utilities to interact with a Pulumi infrastructure
package infra

// RetryType is an enum to specify the type of retry to perform
type RetryType string

const (
	ReUp     RetryType = "ReUp"     // ReUp retries the up operation
	ReCreate RetryType = "ReCreate" // ReCreate retries the up operation after destroying the stack
	NoRetry  RetryType = "NoRetry"  // NoRetry does not retry the up operation
)

type knownError struct {
	errorMessage string
	retryType    RetryType
	maxRetry     int
}

func getKnownErrors() []knownError {
	// Add here errors that are known to be flakes and that should be retried
	return []knownError{
		{
			errorMessage: `i\/o timeout`,
			retryType:    ReCreate,
			maxRetry:     stackUpMaxRetry,
		},
		{
			// https://datadoghq.atlassian.net/browse/ADXT-1
			errorMessage: `failed attempts: dial tcp :22: connect: connection refused`,
			retryType:    ReCreate,
			maxRetry:     stackUpMaxRetry,
		},
		{
			// https://datadoghq.atlassian.net/browse/ADXT-295
			errorMessage: `Resource provider reported that the resource did not exist while updating`,
			retryType:    ReCreate,
			maxRetry:     stackUpMaxRetry,
		},
		{
			// https://datadoghq.atlassian.net/browse/ADXT-558
			// https://datadoghq.atlassian.net/browse/ADXT-713
			errorMessage: `Process exited with status \d+: running " sudo cloud-init status --wait"`,
			retryType:    ReCreate,
			maxRetry:     stackUpMaxRetry,
		},
		{
			errorMessage: `waiting for ECS Service .+fakeintake-ecs.+ create: timeout while waiting for state to become 'tfSTABLE'`,
			retryType:    ReCreate,
			maxRetry:     stackUpMaxRetry,
		},
		{
			errorMessage: `error while waiting for fakeintake`,
			retryType:    ReCreate,
			maxRetry:     stackUpMaxRetry,
		},
		{
			errorMessage: `ssh: handshake failed: ssh: unable to authenticate`,
			retryType:    ReCreate,
			maxRetry:     stackUpMaxRetry,
		},
		{
			// https://datadoghq.atlassian.net/browse/ADXT-798
			// https://datadoghq.atlassian.net/browse/ADXT-813
			errorMessage: `error: awsx:ecs:FargateTaskDefinition resource '.+fakeintake.+' has a problem: grpc: the client connection is closing`,
			retryType:    ReCreate,
			maxRetry:     stackUpMaxRetry,
		},
		{
			// https://datadoghq.atlassian.net/browse/ADXT-726
			errorMessage: `error: .*ssh: rejected: connect failed (No route to host)`,
			retryType:    ReCreate,
			maxRetry:     stackUpMaxRetry,
		},
		{
			errorMessage: `error during container init: error setting cgroup config for procHooks process: unable to freeze: unknown`,
			retryType:    ReUp,
			maxRetry:     5,
		},
	}
}

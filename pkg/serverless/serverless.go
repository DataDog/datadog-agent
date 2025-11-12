// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package serverless

import (
	"strings"
	"time"
)

// ShutdownReason is an AWS Shutdown reason
type ShutdownReason string

// RuntimeEvent is an AWS Runtime event
type RuntimeEvent string

// ErrorEnum are errors reported to the AWS Extension environment.
type ErrorEnum string

// String returns the string value for this ErrorEnum.
func (e ErrorEnum) String() string {
	return string(e)
}

// String returns the string value for this ShutdownReason.
func (s ShutdownReason) String() string {
	return string(s)
}

// Payload is the payload read in the response while subscribing to
// the AWS Extension env.
type Payload struct {
	EventType          RuntimeEvent   `json:"eventType"`
	DeadlineMs         int64          `json:"deadlineMs"`
	InvokedFunctionArn string         `json:"invokedFunctionArn"`
	ShutdownReason     ShutdownReason `json:"shutdownReason"`
	RequestID          string         `json:"requestId"`
}

// FlushableAgent allows flushing
type FlushableAgent interface {
	Flush()
}

func computeTimeout(now time.Time, deadlineMs int64, safetyBuffer time.Duration) time.Duration {
	currentTimeInMs := now.UnixNano() / int64(time.Millisecond)
	return time.Duration((deadlineMs-currentTimeInMs)*int64(time.Millisecond) - int64(safetyBuffer))
}

func removeQualifierFromArn(functionArn string) string {
	functionArnTokens := strings.Split(functionArn, ":")
	tokenLength := len(functionArnTokens)

	if tokenLength > 7 {
		functionArnTokens = functionArnTokens[:tokenLength-1]
		return strings.Join(functionArnTokens, ":")
	}
	return functionArn
}

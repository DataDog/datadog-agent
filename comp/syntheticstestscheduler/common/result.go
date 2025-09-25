// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import "github.com/DataDog/datadog-agent/pkg/networkpath/payload"

// Assertion represents a validation check comparing expected and actual values.
type Assertion struct {
	Operator string      `json:"operator"`
	Type     string      `json:"type"`
	Expected interface{} `json:"expected"`
	Actual   interface{} `json:"actual"`
	Valid    bool        `json:"valid"`
}

// Request represents the network request.
type Request struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	MaxTTL  int    `json:"maxTtl"`
	Timeout int    `json:"timeout"`
}

// NetStats contains aggregated network statistics such as latency and jitter.
type NetStats struct {
	PacketsSent          int                        `json:"packetsSent"`
	PacketsReceived      int                        `json:"packetsReceived"`
	PacketLossPercentage float32                    `json:"packetLossPercentage"`
	Jitter               float64                    `json:"jitter"`
	Latency              payload.E2eProbeRttLatency `json:"latency"`
	Hops                 payload.HopCountStats      `json:"hops"`
}

// Result represents the outcome of a test run including assertions and stats.
type Result struct {
	ID              string              `json:"id"`
	InitialID       string              `json:"initialId"`
	TestFinishedAt  int64               `json:"testFinishedAt"`
	TestStartedAt   int64               `json:"testStartedAt"`
	TestTriggeredAt int64               `json:"testTriggeredAt"`
	Assertions      []Assertion         `json:"assertions"`
	Failure         ErrorOrFailure      `json:"failure"`
	Duration        int64               `json:"duration"`
	Request         Request             `json:"request"`
	Netstats        NetStats            `json:"netstats"`
	Netpath         payload.NetworkPath `json:"netpath"`
	Status          string              `json:"status"`
}

// Test represents the definition of a test including metadata and version.
type Test struct {
	ID      string `json:"id"`
	SubType string `json:"subType"`
	Type    string `json:"type"`
	Version int    `json:"version"`
}

// TestResult represents the full test execution result including metadata.
type TestResult struct {
	DD     map[string]interface{} `json:"_dd"` // TestRequestInternalFields
	Result Result                 `json:"result"`
	Test   Test                   `json:"test"`
	V      int                    `json:"v"` // Major result version
}

// APIErrorCode represents a specific error code returned by the API.
type APIErrorCode string

// APIFailureCode represents a specific failure code returned by the API.
type APIFailureCode string

// APIError represents an API error with a code and message.
type APIError struct {
	Code    APIErrorCode `json:"code"`
	Message string       `json:"message"`
}

// APIFailure represents an API failure with a code and message.
type APIFailure struct {
	Code    APIFailureCode `json:"code"`
	Message string         `json:"message"`
}

// ErrorOrFailure represents an interface for distinguishing errors and failures.
type ErrorOrFailure interface {
	IsError() bool
	IsFailure() bool
}

// IsError returns true.
func (APIError) IsError() bool { return true }

// IsFailure returns false.
func (APIError) IsFailure() bool { return false }

// IsError returns false.
func (APIFailure) IsError() bool { return false }

// IsFailure returns true.
func (APIFailure) IsFailure() bool { return true }

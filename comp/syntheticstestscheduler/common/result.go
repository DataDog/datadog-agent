// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"fmt"
	"reflect"
)

// NetpathSource represents the source host of a network path.
type NetpathSource struct {
	Hostname string `json:"hostname"`
}

// NetpathDestination represents the destination host of a network path.
type NetpathDestination struct {
	Hostname           string `json:"hostname"`
	IPAddress          string `json:"ip_address"`
	Port               int    `json:"port"`
	ReverseDNSHostname string `json:"reverse_dns_hostname"`
}

// NetpathHop represents a single hop in a traceroute path.
type NetpathHop struct {
	TTL       int     `json:"ttl"`
	RTT       float64 `json:"rtt"`
	IPAddress string  `json:"ip_address"`
	Hostname  string  `json:"hostname"`
	Reachable bool    `json:"reachable"`
}

// TracerouteRun represents the result of one traceroute attempt.
type TracerouteRun struct {
	Hops []NetpathHop `json:"hops"`
}

// TracerouteTest aggregates multiple traceroute runs and hop statistics.
type TracerouteTest struct {
	TracerouteRuns []TracerouteRun `json:"traceroute_runs"`
	HopCountAvg    float64         `json:"hop_count_avg"`
	HopCountMin    int             `json:"hop_count_min"`
	HopCountMax    int             `json:"hop_count_max"`
}

// E2ETest represents end-to-end test results such as latency and jitter.
type E2ETest struct {
	PacketLoss float64 `json:"packet_loss"`
	LatencyAvg float64 `json:"latency_avg"`
	LatencyMin float64 `json:"latency_min"`
	LatencyMax float64 `json:"latency_max"`
	Jitter     float64 `json:"jitter"`
}

// NetpathResult represents the full result of a network path test.
type NetpathResult struct {
	Timestamp    int64              `json:"timestamp"`
	PathtraceID  string             `json:"pathtrace_id"`
	Origin       string             `json:"origin"`
	Protocol     string             `json:"protocol"`
	AgentVersion string             `json:"agent_version"`
	Namespace    string             `json:"namespace"`
	Source       NetpathSource      `json:"source"`
	Destination  NetpathDestination `json:"destination"`
	Hops         []NetpathHop       `json:"hops"`
	TestConfigID string             `json:"test_config_id"`
	TestResultID string             `json:"test_result_id"`
	Traceroute   TracerouteTest     `json:"traceroute_test"`
	E2E          E2ETest            `json:"e2e_test"`
	Tags         []string           `json:"tags"`
}

// AssertionResult represents a validation check comparing expected and actual values.
type AssertionResult struct {
	Operator Operator         `json:"operator"`
	Type     AssertionType    `json:"type"`
	Property AssertionSubType `json:"property"`
	Expected interface{}      `json:"expected"`
	Actual   interface{}      `json:"actual"`
	Valid    bool             `json:"valid"`
	Failure  APIFailure       `json:"failure"`
}

// Compare runs the assertion logic
func (a *AssertionResult) Compare() error {
	expectedVal := reflect.ValueOf(a.Expected)
	actualVal := reflect.ValueOf(a.Actual)

	// Convert both to float64 if possible (so we can handle both int & float)
	var exp, act float64
	switch expectedVal.Kind() {
	case reflect.Int, reflect.Int64:
		exp = float64(expectedVal.Int())
	case reflect.Float32, reflect.Float64:
		exp = expectedVal.Float()
	default:
		return fmt.Errorf("expected must be int or float")
	}

	switch actualVal.Kind() {
	case reflect.Int, reflect.Int64:
		act = float64(actualVal.Int())
	case reflect.Float32, reflect.Float64:
		act = actualVal.Float()
	default:
		return fmt.Errorf("actual must be int or float")
	}

	switch a.Operator {
	case OperatorLessThan:
		a.Valid = act < exp
	case OperatorLessThanOrEquals:
		a.Valid = act <= exp
	case OperatorMoreThan:
		a.Valid = act > exp
	case OperatorMoreThanOrEquals:
		a.Valid = act >= exp
	case OperatorIs:
		a.Valid = act == exp
	case OperatorIsNot:
		a.Valid = act != exp
	default:
		return fmt.Errorf("unsupported operator")
	}

	return nil
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
	PacketsSent          int     `json:"packetsSent"`
	PacketsReceived      int     `json:"packetsReceived"`
	PacketLossPercentage float64 `json:"packetLossPercentage"`
	Jitter               float64 `json:"jitter"`
	Latency              struct {
		Avg float64 `json:"avg"`
		Min float64 `json:"min"`
		Max float64 `json:"max"`
	} `json:"latency"`
	Hops struct {
		Avg float64 `json:"avg"`
		Min int     `json:"min"`
		Max int     `json:"max"`
	} `json:"hops"`
}

// Result represents the outcome of a test run including assertions and stats.
type Result struct {
	ID              string            `json:"id"`
	InitialID       string            `json:"initialId"`
	TestFinishedAt  int64             `json:"testFinishedAt"`
	TestStartedAt   int64             `json:"testStartedAt"`
	TestTriggeredAt int64             `json:"testTriggeredAt"`
	Assertions      []AssertionResult `json:"assertions"`
	Failure         ErrorOrFailure    `json:"failure"`
	Duration        int64             `json:"duration"`
	Request         Request           `json:"request"`
	Netstats        NetStats          `json:"netstats"`
	Netpath         NetpathResult     `json:"netpath"`
	Status          string            `json:"status"`
}

// Test represents the definition of a test including metadata and version.
type Test struct {
	InternalID string `json:"_internalId"`
	ID         string `json:"id"`
	SubType    string `json:"subType"`
	Type       string `json:"type"`
	Version    int    `json:"version"`
}

// TestResult represents the full test execution result including metadata.
type TestResult struct {
	DD     map[string]interface{} `json:"_dd"`
	Result Result                 `json:"result"`
	Test   Test                   `json:"test"`
	V      int                    `json:"v"`
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

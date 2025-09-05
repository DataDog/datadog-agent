// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

type NetpathSource struct {
	Hostname string `json:"hostname"`
}

type NetpathDestination struct {
	Hostname           string `json:"hostname"`
	IPAddress          string `json:"ip_address"`
	Port               int    `json:"port"`
	ReverseDNSHostname string `json:"reverse_dns_hostname"`
}

type NetpathHop struct {
	TTL       int     `json:"ttl"`
	RTT       float64 `json:"rtt"`
	IPAddress string  `json:"ip_address"`
	Hostname  string  `json:"hostname"`
	Reachable bool    `json:"reachable"`
}

type TracerouteRun struct {
	Hops []NetpathHop `json:"hops"`
}

type TracerouteTest struct {
	TracerouteRuns []TracerouteRun `json:"traceroute_runs"`
	HopCountAvg    float64         `json:"hop_count_avg"`
	HopCountMin    int             `json:"hop_count_min"`
	HopCountMax    int             `json:"hop_count_max"`
}

type E2ETest struct {
	PacketLoss float64 `json:"packet_loss"`
	LatencyAvg float64 `json:"latency_avg"`
	LatencyMin float64 `json:"latency_min"`
	LatencyMax float64 `json:"latency_max"`
	Jitter     float64 `json:"jitter"`
}

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

type Assertion struct {
	Operator string      `json:"operator"`
	Type     string      `json:"type"`
	Expected interface{} `json:"expected"`
	Actual   interface{} `json:"actual"`
	Valid    bool        `json:"valid"`
}

type Request struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	MaxTTL  int    `json:"maxTtl"`
	Timeout int    `json:"timeout"`
}

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

type Result struct {
	ID              string         `json:"id"`
	InitialID       string         `json:"initialId"`
	TestFinishedAt  int64          `json:"testFinishedAt"`
	TestStartedAt   int64          `json:"testStartedAt"`
	TestTriggeredAt int64          `json:"testTriggeredAt"`
	Assertions      []Assertion    `json:"assertions"`
	Failure         ErrorOrFailure `json:"failure"`
	Duration        int64          `json:"duration"`
	Request         Request        `json:"request"`
	Netstats        NetStats       `json:"netstats"`
	Netpath         NetpathResult  `json:"netpath"`
	Status          string         `json:"status"`
}

type Test struct {
	InternalID string `json:"_internalId"`
	ID         string `json:"id"`
	SubType    string `json:"subType"`
	Type       string `json:"type"`
	Version    int    `json:"version"`
}

type TestResult struct {
	DD     map[string]interface{} `json:"_dd"`
	Result Result                 `json:"result"`
	Test   Test                   `json:"test"`
	V      int                    `json:"v"`
}

type APIErrorCode string
type APIFailureCode string

type APIError struct {
	Code    APIErrorCode `json:"code"`
	Message string       `json:"message"`
}

type APIFailure struct {
	Code    APIFailureCode `json:"code"`
	Message string         `json:"message"`
}

type ErrorOrFailure interface {
	IsError() bool
	IsFailure() bool
}

func (_ APIError) IsError() bool {
	return true
}
func (_ APIError) IsFailure() bool {
	return false
}
func (_ APIFailure) IsFailure() bool {
	return true
}
func (_ APIFailure) IsError() bool {
	return false
}

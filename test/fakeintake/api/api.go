// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package api TODO comment
package api

import "time"

// Payload exported type should have comment or be unexported
type Payload struct {
	Timestamp time.Time `json:"timestamp"`
	Data      []byte    `json:"data"`
	Encoding  string    `json:"encoding"`
}

// APIFakeIntakePayloadsGETResponse exported type should have comment or be unexported
type APIFakeIntakePayloadsGETResponse struct {
	Payloads []Payload `json:"payloads"`
}

// RouteStat exported type should have comment or be unexported
type RouteStat struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

// APIFakeIntakeRouteStatsGETResponse exported type should have comment or be unexported
type APIFakeIntakeRouteStatsGETResponse struct {
	Routes map[string]RouteStat `json:"routes"`
}

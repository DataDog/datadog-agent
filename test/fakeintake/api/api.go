// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import "time"

type Payload struct {
	Timestamp time.Time `json:"timestamp"`
	Data      []byte    `json:"data"`
	Encoding  string    `json:"encoding"`
}

type APIFakeIntakePayloadsGETResponse struct {
	Payloads []Payload `json:"payloads"`
}

type RouteStat struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

type APIFakeIntakeRouteStatsGETResponse struct {
	Routes map[string]RouteStat `json:"routes"`
}

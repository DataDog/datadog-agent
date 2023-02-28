// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

type APIFakeIntakePayloadsGETResponse struct {
	Payloads [][]byte `json:"payloads"`
}

type RouteStat struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

type APIFakeIntakeRouteStatsGETResponse struct {
	Routes map[string]RouteStat `json:"routes"`
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(APL) Fix revive linter
package api

import "time"

//nolint:revive // TODO(APL) Fix revive linter
type Payload struct {
	Timestamp time.Time `json:"timestamp"`
	Data      []byte    `json:"data"`
	Encoding  string    `json:"encoding"`
}

//nolint:revive // TODO(APL) Fix revive linter
type ParsedPayload struct {
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
	Encoding  string      `json:"encoding"`
}

//nolint:revive // TODO(APL) Fix revive linter
type APIFakeIntakePayloadsRawGETResponse struct {
	Payloads []Payload `json:"payloads"`
}

//nolint:revive // TODO(APL) Fix revive linter
type APIFakeIntakePayloadsJsonGETResponse struct {
	Payloads []ParsedPayload `json:"payloads"`
}

//nolint:revive // TODO(APL) Fix revive linter
type RouteStat struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

//nolint:revive // TODO(APL) Fix revive linter
type APIFakeIntakeRouteStatsGETResponse struct {
	Routes map[string]RouteStat `json:"routes"`
}

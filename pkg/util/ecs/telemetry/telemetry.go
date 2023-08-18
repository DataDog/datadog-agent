// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package telemetry

import (
	"net/http"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	// queries tracks ECS requests done by the Agent.
	queries = telemetry.NewCounterWithOpts(
		"ecs",
		"queries",
		[]string{"path", "code"},
		"Count of ECS queries by path and response code. The response code defaults to 0 for unachieved queries.",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
)

// AddQueryToTelemetry Adds a query to the telemetry
func AddQueryToTelemetry(path string, response *http.Response) {
	code := 0
	if response != nil {
		code = response.StatusCode
	}

	queries.Inc(path, strconv.Itoa(code))
}

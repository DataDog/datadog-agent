// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package debugger

import (
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	"net/http"
)

func GetHTTPDebugEndpoint(tracer *tracer.Tracer) func(http.ResponseWriter, *http.Request) {
	return nil
}

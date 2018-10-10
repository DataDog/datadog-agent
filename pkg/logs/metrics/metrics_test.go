// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetrics(t *testing.T) {
	assert.Equal(t, logsExpvars.String(), `{"DestinationErrors": 0, "IsRunning": false, "LogsDecoded": 0, "LogsProcessed": 0, "LogsSent": 0, "Warnings": ""}`)
}

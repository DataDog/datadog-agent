// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetrics(t *testing.T) {
	assert.Equal(t, LogsExpvars.String(), `{"BytesSent": 0, "DestinationErrors": 0, "DestinationLogsDropped": {}, "EncodedBytesSent": 0, "LogsDecoded": 0, "LogsProcessed": 0, "LogsSent": 0}`)
}

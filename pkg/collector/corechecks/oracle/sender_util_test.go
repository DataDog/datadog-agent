// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSendMetricDoesNotPanicOnInvalidMetricType(t *testing.T) {
	chk, _ := newDbDoesNotExistCheck(t, "", "")
	chk.Run()

	assert.NotPanics(t, func() {
		sendMetric(&chk, unknownMetricType, "oracle.test.metric", 1.0, []string{"tag:value"})
	}, "sendMetric should not panic when given an invalid metric type")
}

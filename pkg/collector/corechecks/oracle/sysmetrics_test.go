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

func TestSysmetrics(t *testing.T) {
	c, _ := newDefaultCheck(t, "dbm: true", "")
	c.Run()
	assert.True(t, c.dbmEnabled, "dbm should be enabled")
	n, err := c.sysMetrics()
	assert.NoError(t, err, "failed to run sys metrics")
	var expected int64
	if c.connectedToPdb {
		expected = 66
	} else {
		expected = 92
	}
	assert.Equal(t, expected, n)
}

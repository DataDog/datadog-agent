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
	chk, _ := newRealCheck(t, "dbm: true")
	chk.Run()
	assert.True(t, chk.dbmEnabled, "dbm should be enabled")
	n, err := chk.sysMetrics()
	assert.NoError(t, err, "failed to run sys metrics")
	assert.Equal(t, int64(92), n)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"testing"

	"github.com/stretchr/testify/assert"

	_ "github.com/godror/godror"
)

func TestGetFullSqlText(t *testing.T) {
	c, _ := newDefaultCheck(t, "", "")
	c.db = nil

	c.dbmEnabled = false

	err := c.Run()
	assert.NoError(t, err, "check run")

	var SQLStatement string
	err = getFullSQLText(&c, &SQLStatement, "sql_id", "A")
	assert.NoError(t, err, "no rows returned an error")
}

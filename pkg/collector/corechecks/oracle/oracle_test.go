// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"testing"

	_ "github.com/godror/godror"
	"github.com/stretchr/testify/assert"
)

func TestDbmDefault(t *testing.T) {
	c, _ := newDbDoesNotExistCheck(t, "", "")
	defer c.Teardown()
	assert.Falsef(t, c.dbmEnabled, "Defaul dbm = false")
}

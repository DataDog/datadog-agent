// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
	"github.com/stretchr/testify/assert"

	_ "github.com/godror/godror"
)

func TestGetFullSqlText(t *testing.T) {
	initAndStartAgentDemultiplexer(t)
	for _, tnsAlias := range []string{"", TNS_ALIAS} {
		chk.db = nil

		chk.config.InstanceConfig.TnsAlias = tnsAlias
		chk.dbmEnabled = false

		var driver string
		if tnsAlias == "" {
			driver = common.GoOra
			chk.config.InstanceConfig.InstantClient = false
		} else {
			driver = common.Godror
		}
		chk.Run()

		var SQLStatement string
		err := getFullSQLText(&chk, &SQLStatement, "sql_id", "A")
		assert.NoError(t, err, "no rows returned an error with %s driver", driver)
	}
}

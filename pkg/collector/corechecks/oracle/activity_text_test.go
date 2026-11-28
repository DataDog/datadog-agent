// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestActivityFetchesFullSQLUsingClobLength(t *testing.T) {
	// A clipped prefix must not produce a different signature just because its UTF-8 size differs from Oracle's limit.
	for _, previous := range []bool{false, true} {
		for _, char := range []string{"x", "é", "漢", "😀", "e\u0301"} {
			t.Run(fmt.Sprintf("previous=%t/%s", previous, char), func(t *testing.T) {
				db, mock, err := sqlmock.New()
				require.NoError(t, err)
				defer db.Close()

				c := newCheck().(*Check)
				manager := mocksender.CreateDefaultDemultiplexer(t)
				raw := integration.Data("server: localhost\nport: 1521\nservice_name: XE\nusername: test\npassword: test\n")
				require.NoError(t, c.Configure(manager, integration.FakeConfigHash, raw, nil, "test", ""))
				sender := mocksender.NewMockSenderWithSenderManager(c.ID(), manager)
				sender.SetupAcceptAll()
				c.db = sqlx.NewDb(db, "sqlmock")
				c.driver = common.Godror
				c.sqlSubstringLength = 32
				c.config.QuerySamples.ForceDirectQuery = true
				c.config.QuerySamples.IncludeAllSessions = true

				prefix := "SELECT /*" + strings.Repeat(char, 23)
				if char == "😀" {
					prefix = "SELECT /*" + strings.Repeat(char, 11)
				} else if char == "e\u0301" {
					prefix = "SELECT /*" + strings.Repeat(char, 11) + "e"
				}
				full := prefix + "*/ 1"
				columns := []string{"NOW", "SID", "STATUS", "SQL_ID", "SQL_FULLTEXT", "SQL_FULLTEXT_LENGTH"}
				if previous {
					columns = []string{"NOW", "SID", "STATUS", "PREV_SQL_ID", "PREV_SQL_FULLTEXT", "PREV_SQL_FULLTEXT_LENGTH"}
				}
				mock.ExpectQuery("DD_ACTIVITY_SAMPLING").WillReturnRows(sqlmock.NewRows(columns).
					AddRow("2026-11-19 00:00:00", 100, "ACTIVE", "test_sql_id", prefix, utf8.RuneCountInString(full)))
				mock.ExpectQuery("SELECT .* sql_fulltext FROM v\\$sql").WithArgs("test_sql_id").
					WillReturnRows(sqlmock.NewRows([]string{"SQL_FULLTEXT"}).AddRow(full))

				require.NoError(t, c.SampleSession())
				require.Len(t, c.lastOracleActivityRows, 1)
				expected, err := c.GetObfuscatedStatement(c.LazyInitObfuscator(), full)
				require.NoError(t, err)
				require.Equal(t, expected.Statement, c.lastOracleActivityRows[0].Statement)
				require.Equal(t, expected.QuerySignature, c.lastOracleActivityRows[0].QuerySignature)
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

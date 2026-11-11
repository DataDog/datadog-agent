// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestFullSQLTextUsesMainPool(t *testing.T) {
	queryError := errors.New("ORA-00942: table or view does not exist")
	fetchError := errors.New("failed to fetch SQL text")
	fullText := "SELECT /* " + strings.Repeat("long_sql_é", 2000) + " */ 1 FROM dual"
	for _, driver := range []string{common.GoOra, common.Godror} {
		t.Run(driver, func(t *testing.T) {
			for _, tc := range []struct {
				name     string
				text     string
				noRows   bool
				queryErr error
				fetchErr error
			}{
				{name: "complete long text", text: fullText},
				{name: "evicted SQL", noRows: true},
				{name: "query failure", queryErr: queryError},
				{name: "fetch failure", fetchErr: fetchError},
			} {
				t.Run(tc.name, func(t *testing.T) {
					db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
					require.NoError(t, err)
					t.Cleanup(func() { _ = db.Close() })
					c := &Check{db: sqlx.NewDb(db, driver), driver: driver}
					pool := c.db
					query := mock.ExpectQuery("SELECT /* DD */ sql_fulltext FROM v$sql WHERE sql_id = :v AND rownum = 1").WithArgs("test_sql_id")
					if tc.queryErr != nil {
						query.WillReturnError(tc.queryErr)
					} else {
						rows := sqlmock.NewRows([]string{"SQL_FULLTEXT"})
						if !tc.noRows {
							rows.AddRow(tc.text)
						}
						if tc.fetchErr != nil {
							rows.RowError(0, tc.fetchErr)
						}
						query.WillReturnRows(rows).RowsWillBeClosed()
					}

					text := "existing truncated text"
					err = getFullSQLText(c, &text, "sql_id", "test_sql_id")
					switch {
					case tc.queryErr != nil:
						require.ErrorIs(t, err, tc.queryErr)
					case tc.fetchErr != nil:
						require.ErrorIs(t, err, tc.fetchErr)
					default:
						require.NoError(t, err)
					}
					if tc.noRows || tc.queryErr != nil || tc.fetchErr != nil {
						require.Equal(t, "existing truncated text", text)
					} else {
						require.Equal(t, fullText, text)
					}
					require.Same(t, pool, c.db)
					mock.ExpectQuery("SELECT 1 FROM dual").WillReturnRows(sqlmock.NewRows([]string{"VALUE"}).AddRow(1)).RowsWillBeClosed()
					var one int
					require.NoError(t, c.db.Get(&one, "SELECT 1 FROM dual"))
					require.Equal(t, 1, one)
					require.NoError(t, mock.ExpectationsWereMet())
				})
			}
		})
	}
}

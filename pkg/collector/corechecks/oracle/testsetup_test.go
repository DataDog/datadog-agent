// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"errors"
	"os"
	"regexp"
	"testing"
	"testing/fstest"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestWaitForTestDatabase(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	query := regexp.QuoteMeta(testDatabaseReadyQuery)
	mock.ExpectQuery(query).WillReturnError(errors.New("ORA-12514"))
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"ready"}).AddRow(0))
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"ready"}).AddRow(1))

	retry := make(chan time.Time, 2)
	retry <- time.Time{}
	retry <- time.Time{}
	require.NoError(t, waitForTestDatabase(context.Background(), db, retry))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWaitForTestDatabaseCancellation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, waitForTestDatabase(ctx, db, nil), context.Canceled)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWaitForTestDatabaseQueryCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		cancel()
		return sqlmock.QueryMatcherRegexp.Match(expected, actual)
	})
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(testDatabaseReadyQuery)).
		WillDelayFor(time.Hour).
		WillReturnRows(sqlmock.NewRows([]string{"ready"}).AddRow(1))

	require.ErrorIs(t, waitForTestDatabase(ctx, db, nil), context.Canceled)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInitializeTestDatabase(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	block := "BEGIN\n  NULL;\nEND;"
	scripts := fstest.MapFS{
		"00-user.sql":          {Data: []byte("  -- comment\nCREATE USER test;\n\n ;\nGRANT CREATE SESSION TO test;\n")},
		"01-block.nosplit.sql": {Data: []byte(block)},
		"README.md":            {Data: []byte("not SQL")},
		"subdir/ignored.sql":   {Data: []byte("not executed")},
	}
	mock.ExpectExec("CREATE USER test").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("GRANT CREATE SESSION TO test").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(block)).WillReturnResult(sqlmock.NewResult(0, 0))

	require.NoError(t, initializeTestDatabase(context.Background(), db, scripts))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInitializeTestDatabaseStopsOnError(t *testing.T) {
	for _, filename := range []string{"00-user.sql", "00-block.nosplit.sql"} {
		t.Run(filename, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			failure := errors.New("ORA-01031: insufficient privileges")
			scripts := fstest.MapFS{
				filename:       {Data: []byte("CREATE USER test")},
				"01-later.sql": {Data: []byte("SELECT 1 FROM dual")},
			}
			mock.ExpectExec("CREATE USER test").WillReturnError(failure)
			mock.ExpectExec("SELECT 1 FROM dual").WillReturnResult(sqlmock.NewResult(0, 0))

			err = initializeTestDatabase(context.Background(), db, scripts)
			require.ErrorIs(t, err, failure)
			require.ErrorContains(t, err, filename)
			require.Error(t, mock.ExpectationsWereMet(), "later script must not execute")
		})
	}
}

func TestInitializeTestDatabaseMissingScripts(t *testing.T) {
	err := initializeTestDatabase(context.Background(), nil, os.DirFS(t.TempDir()+"/missing"))
	require.ErrorContains(t, err, "reading initialization scripts")
}

func TestSetupTestDatabaseInvalidTimeout(t *testing.T) {
	for _, timeout := range []string{"invalid", "0s", "-1s"} {
		t.Run(timeout, func(t *testing.T) {
			t.Setenv("ORACLE_TEST_READY_TIMEOUT", timeout)
			require.ErrorContains(t, setupTestDatabase(), "must be a positive duration")
		})
	}
}

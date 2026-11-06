// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestSchemaDetailRelations(t *testing.T) {
	key := tableKey{conID: 3, owner: "APP", table: "T"}
	for _, test := range []struct {
		name                  string
		rows                  []schemaRowDB
		external, partitioned bool
	}{
		{"known ordinary", []schemaRowDB{{External: "NO", Partitioned: "NO"}}, false, false},
		{"external", []schemaRowDB{{External: "YES", Partitioned: "NO"}}, true, false},
		{"partitioned", []schemaRowDB{{External: "NO", Partitioned: "YES"}}, false, true},
		{"both", []schemaRowDB{{External: "YES", Partitioned: "YES"}}, true, true},
		{"blank", []schemaRowDB{{}}, true, true},
		{"unknown", []schemaRowDB{{External: "?", Partitioned: "?"}}, true, true},
		{"missing", nil, true, true},
		{"zero columns", []schemaRowDB{{External: "NO", Partitioned: "NO", ColumnPresent: sql.NullInt64{Int64: 0, Valid: true}}}, false, false},
		{"conflicting", []schemaRowDB{{External: "NO", Partitioned: "NO"}, {External: "YES", Partitioned: "YES"}}, true, true},
		{"unknown after no", []schemaRowDB{{External: "NO", Partitioned: "NO"}, {}}, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			for i := range test.rows {
				test.rows[i].ConID, test.rows[i].Owner, test.rows[i].TableName = key.conID, key.owner, key.table
			}
			allowed := map[tableKey]struct{}{key: {}}
			external, partitioned := schemaDetailRelations(test.rows, allowed)
			_, hasExternal := external[key]
			_, hasPartitioned := partitioned[key]
			require.Equal(t, test.external, hasExternal)
			require.Equal(t, test.partitioned, hasPartitioned)
			require.Equal(t, map[tableKey]struct{}{key: {}}, allowed)
		})
	}
}

func TestSchemaDetailRelationsFullKeys(t *testing.T) {
	ordinary := tableKey{conID: 3, owner: "APP", table: "SAME"}
	external := tableKey{conID: 3, owner: "OTHER", table: "SAME"}
	partitioned := tableKey{conID: 4, owner: "APP", table: "SAME"}
	allowed := map[tableKey]struct{}{ordinary: {}, external: {}, partitioned: {}}
	rows := []schemaRowDB{
		{ConID: ordinary.conID, Owner: ordinary.owner, TableName: ordinary.table, External: "NO", Partitioned: "NO"},
		{ConID: external.conID, Owner: external.owner, TableName: external.table, External: "YES", Partitioned: "NO"},
		{ConID: partitioned.conID, Owner: partitioned.owner, TableName: partitioned.table, External: "NO", Partitioned: "YES"},
		{ConID: 9, Owner: "UNSELECTED", TableName: "SAME", External: "YES", Partitioned: "YES"},
	}
	actualExternal, actualPartitioned := schemaDetailRelations(rows, allowed)
	require.Equal(t, map[tableKey]struct{}{external: {}}, actualExternal)
	require.Equal(t, map[tableKey]struct{}{partitioned: {}}, actualPartitioned)
	require.Len(t, allowed, 3)
}

func TestSchemaOrderedWindows(t *testing.T) {
	for _, workers := range []int{2, 4} {
		t.Run(strconv.Itoa(workers), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			secondStarted := make(chan struct{})
			var observed []string
			err := schemaOrderedWindows(ctx, 8, workers, func(ctx context.Context, _, window int, emit func(schemaEvent) error) error {
				if window == 1 {
					close(secondStarted)
				}
				if window == 0 {
					select {
					case <-secondStarted:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				for page := range 3 {
					if err := emit(schemaEvent{Host: fmt.Sprintf("%d/%d", window, page)}); err != nil {
						return err
					}
				}
				return nil
			}, func(event schemaEvent) error {
				observed = append(observed, event.Host)
				return nil
			})
			require.NoError(t, err)
			var expected []string
			for window := range 8 {
				for page := range 3 {
					expected = append(expected, fmt.Sprintf("%d/%d", window, page))
				}
			}
			require.Equal(t, expected, observed)
		})
	}
}

func TestSchemaBufferedWindowProgress(t *testing.T) {
	for _, workers := range []int{2, 4} {
		t.Run(strconv.Itoa(workers), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			laterFinished := make(chan struct{})
			var observed []string
			err := schemaOrderedWindowsBuffered(ctx, workers, workers, 4<<20, func(ctx context.Context, _, window int, emit func(schemaEvent) error) error {
				if window == 0 {
					select {
					case <-laterFinished:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				for page := range 3 {
					if err := emit(schemaEvent{Host: fmt.Sprintf("%d/%d", window, page)}); err != nil {
						return err
					}
				}
				if window == 1 {
					close(laterFinished)
				}
				return nil
			}, func(event schemaEvent) error {
				observed = append(observed, event.Host)
				return nil
			})
			require.NoError(t, err)
			var expected []string
			for window := range workers {
				for page := range 3 {
					expected = append(expected, fmt.Sprintf("%d/%d", window, page))
				}
			}
			require.Equal(t, expected, observed)
		})

	}
}

func TestSchemaBufferedWindowBudget(t *testing.T) {
	for _, cancelRun := range []bool{false, true} {
		for _, oversized := range []bool{false, true} {
			t.Run(fmt.Sprintf("cancel=%v/oversized=%v", cancelRun, oversized), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					event := schemaEvent{Host: strings.Repeat("x", 4096)}
					encoded, err := json.Marshal(event)
					require.NoError(t, err)
					budget, expected := int64(2*cap(encoded)), int64(2)
					if oversized {
						budget, expected = 1, 1
					}
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					releaseHead := make(chan struct{})
					var sent atomic.Int64
					done := make(chan error, 1)
					go func() {
						done <- schemaOrderedWindowsBuffered(ctx, 2, 2, budget, func(ctx context.Context, _, window int, emit func(schemaEvent) error) error {
							if window == 0 {
								select {
								case <-releaseHead:
								case <-ctx.Done():
									return ctx.Err()
								}
								return emit(schemaEvent{Host: "head"})
							}
							for range 3 {
								if err := emit(event); err != nil {
									return err
								}
								sent.Add(1)
							}
							return nil
						}, func(schemaEvent) error { return nil })
					}()
					synctest.Wait()
					require.Equal(t, expected, sent.Load(), "byte allowance must stop the producer before the 64-record limit")
					if cancelRun {
						cancel()
						require.ErrorIs(t, <-done, context.Canceled)
					} else {
						close(releaseHead)
						require.NoError(t, <-done)
						require.EqualValues(t, 3, sent.Load())
					}
				})
			})
		}
	}
}

func TestSchemaBufferedEventContract(t *testing.T) {
	zero := int64(0)
	full := &schemaTable{
		Name: "TABLE", Owner: "OWNER", TableType: "materialized_view", Properties: []string{},
		Temporary:   &temporaryDetail{Scope: "SYS$SESSION"},
		Partitioned: &partitionDetail{PartitionKey: "A", NumPartitions: 3, PartitioningType: "HASH", SubpartitionsType: "NONE"},
		Blockchain:  &retentionDetail{RowRetentionDays: &zero, RowRetentionLocked: true, InactivityRetentionDays: &zero, HashAlgorithm: "SHA2_512", TableVersion: "V1"},
		Immutable:   &retentionDetail{}, ObjectType: &objectTypeDetail{TypeOwner: "OWNER", TypeName: "TYPE"},
		Comment: strings.Repeat("é<>&", 10000), External: &externalDetail{AccessDriver: "ORACLE_LOADER", Directory: "DIR", Locations: []string{"a", "b"}},
		Mview:       &mviewDetail{RefreshMode: "DEMAND", RefreshMethod: "FAST", Staleness: "FRESH", LastRefreshDate: "2026-10-21"},
		Indexes:     []*indexInfo{{owner: "internal", Name: "IDX", Unique: true, Type: "NORMAL", Columns: []indexKeyPart{{Column: "A"}, {Expression: "UPPER(B)"}}}},
		Constraints: []*constraintInfo{{Name: "FK", Type: "R", Columns: []string{"A"}, Condition: "A > 0", ReferencedOwner: "OWNER", ReferencedTable: "OTHER", ReferencedColumns: []string{"ID"}, ReferencedConstraint: "PK", referencedName: "internal", referencedKey: &constraintKey{conID: 3, owner: "OWNER", name: "PK"}}},
		Columns:     []schemaColumn{{Name: "A", DataType: "VARCHAR2(10)", Nullable: false, Default: strings.Repeat("'unicode é'", 1000), Comment: "column", Virtual: true, Invisible: true}},
	}
	event := schemaEvent{Host: "host", DatabaseInstance: "db", AgentVersion: "agent", Dbms: "oracle", Kind: "oracle_databases", CollectionInterval: 600, DbmsVersion: "19", Tags: []string{"a:b"}, Timestamp: 123.5, CollectionStartedAt: 123, CollectionPayloadsCount: 2, Truncated: true,
		Metadata: []containerObject{{ID: "3", Name: "PDB", Schemas: []*schemaObject{{ID: "owner", Name: "OWNER", Owner: "OWNER", Tables: []*schemaTable{full, {Name: "empty", Properties: []string{}, Constraints: []*constraintInfo{}, Columns: []schemaColumn{}}, {Name: "nil"}}, Views: []*viewObject{{Name: "VIEW", Owner: "OWNER", Definition: "SELECT 1", Comment: "view", CreateDate: "today", ModifyDate: "today", Columns: []schemaColumn{}}}}}}}}
	expected, err := json.Marshal(event)
	require.NoError(t, err)
	var actual []byte
	err = schemaOrderedWindowsBuffered(context.Background(), 1, 2, 1, func(_ context.Context, _, _ int, emit func(schemaEvent) error) error { return emit(event) }, func(decoded schemaEvent) error {
		var err error
		actual, err = json.Marshal(decoded)
		return err
	})
	require.NoError(t, err)
	require.Equal(t, string(expected), string(actual))
}

func TestSchemaBufferedRecordLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var sent atomic.Int64
		done := make(chan error, 1)
		go func() {
			done <- schemaOrderedWindows(ctx, 2, 2, func(ctx context.Context, _, window int, emit func(schemaEvent) error) error {
				if window == 0 {
					<-ctx.Done()
					return ctx.Err()
				}
				for range 100 {
					if err := emit(schemaEvent{Host: "small"}); err != nil {
						return err
					}
					sent.Add(1)
				}
				return nil
			}, func(schemaEvent) error { return nil })
		}()
		synctest.Wait()
		require.EqualValues(t, schemaWindowBufferEvents, sent.Load())
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
	})
}

func TestSchemaWindowFailuresJoin(t *testing.T) {
	for _, workers := range []int{2, 4} {
		for _, mode := range []string{"panic", "query", "emitter", "cancel"} {
			t.Run(fmt.Sprintf("%d/%s", workers, mode), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				var active atomic.Int64
				failure := errors.New("distinct failure")
				err := schemaOrderedWindows(ctx, 20, workers, func(_ context.Context, _, window int, emit func(schemaEvent) error) error {
					active.Add(1)
					defer active.Add(-1)
					if window == 1 {
						switch mode {
						case "panic":
							panic("distinct panic")
						case "query":
							return failure
						case "cancel":
							cancel()
						}
					}
					return emit(schemaEvent{})
				}, func(schemaEvent) error {
					if mode == "emitter" {
						return failure
					}
					return nil
				})
				require.Error(t, err)
				require.Zero(t, active.Load())
				switch mode {
				case "panic":
					require.ErrorContains(t, err, "distinct panic")
				case "query", "emitter":
					require.ErrorIs(t, err, failure)
				case "cancel":
					require.ErrorIs(t, err, context.Canceled)
				}
			})
		}
	}
}

func TestSchemaWindowCheckIsolation(t *testing.T) {
	c, _, _, cleanup := newSchemaCheck(t)
	defer cleanup()
	c.lastSnapshotID = 10
	first := c.schemaWindowCheck(c.schemaQueryer)
	second := c.schemaWindowCheck(c.schemaQueryer)
	first.schemaQueryError = errors.New("first worker")
	require.Nil(t, c.schemaQueryError)
	require.Nil(t, second.schemaQueryError)
	require.EqualValues(t, 10, c.lastSnapshotID)
	require.Zero(t, first.lastSnapshotID)
	require.Zero(t, second.lastSnapshotID)
	require.Nil(t, first.schemaWorkerState)
	require.Nil(t, second.schemaWorkerState)
	require.Nil(t, first.schemaEmitter)
}

func TestSchemaBindingsKeepFullAndPartialPagesStable(t *testing.T) {
	for _, count := range []int{1, schemaRelationPageSize} {
		allowed := make(map[tableKey]struct{})
		for i := range count {
			allowed[tableKey{conID: 4, owner: "OTHER", table: fmt.Sprintf("T%03d", i)}] = struct{}{}
		}
		columns := relationColumnNames{conID: "t.con_id", owner: "t.owner", relation: "t.table_name"}
		full := relationFilterChunks(allowed, columns)
		partial := relationFilterChunks(map[tableKey]struct{}{{conID: 3, owner: "APP", table: "O'RDER"}: {}}, columns)
		require.Len(t, full, 1)
		require.Equal(t, full[0].text, partial[0].text)
		require.Len(t, full[0].args, 102)
		require.NotEqual(t, full[0].args, partial[0].args)
	}
}

func TestSchemaColumnCommentFilterBoundary(t *testing.T) {
	for _, count := range []int{1, 100, 500} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			c, _, mock, cleanup := newSchemaCheck(t)
			defer cleanup()
			allowed := make(map[columnKey]struct{})
			for _, page := range schemaColumnPages(count, 2) {
				for key := range columnKeysFromRows(page) {
					allowed[key] = struct{}{}
				}
			}
			filters := columnFilterChunks(allowed, relationColumnNames{conID: "con_id", owner: "owner", relation: "table_name"}, "column_name")
			require.Len(t, filters, 1)
			query := `SELECT con_id, owner, table_name, column_name, comments FROM (
				SELECT con_id, owner, table_name, column_name, comments
				FROM cdb_col_comments WHERE ` + filters[0].text + ` AND ROWNUM > 0
			) WHERE comments IS NOT NULL`
			mock.ExpectQuery(regexp.QuoteMeta(strings.Join(strings.Fields(query), " "))).
				WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "COMMENTS"}).
					AddRow(3, "APP", "T0000", "C0000", "列の説明").
					AddRow(3, "APP", "T0000", "C0001", " "))
			detail := &tableDetails{}
			c.loadSchemaColumnDetails(context.Background(), filters, nil, func(key columnKey) *tableDetails {
				require.Contains(t, allowed, key)
				return detail
			})
			require.Equal(t, map[string]string{"C0000": "列の説明", "C0001": " "}, detail.ColumnComments)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSchemaBindOrderArguments(t *testing.T) {
	null := sql.NullString{}
	value := sql.NullString{String: "quoted 'value': :not_a_bind", Valid: true}
	query := schemaSQL{
		text: "SELECT :v1, :v10, :v1, :NULL_VALUE, :text_value, ':literal''quoted', 1 AS \"quoted:identifier\" /* :comment */ -- :line\nFROM dual",
		args: []any{sql.Named("v10", int64(10)), sql.Named("v1", int64(1)), sql.Named("null_value", null), sql.Named("text_value", value)},
	}
	ordered, err := schemaBindOrderArguments(query)
	require.NoError(t, err)
	require.Equal(t, []any{int64(1), int64(10), int64(1), null, value}, ordered)
	for _, text := range []string{"SELECT :missing FROM dual", "SELECT ':v1", "SELECT /* :v1", "SELECT q'[x:v1]' FROM dual", "SELECT ::v1 FROM dual", "SELECT 1 FROM dual"} {
		_, err := schemaBindOrderArguments(schemaSQL{text: text, args: []any{sql.Named("v1", 1)}})
		require.Error(t, err, text)
	}
	_, err = schemaBindOrderArguments(schemaSQL{text: "SELECT :v FROM dual", args: []any{sql.Named("v", 1), sql.Named("V", 2)}})
	require.Error(t, err)
	_, err = schemaBindOrderArguments(schemaSQL{text: "SELECT :v FROM dual", args: []any{1}})
	require.Error(t, err)
}

func TestSchemaQueryArgumentsPreserveDriverAndQueryScope(t *testing.T) {
	for _, driver := range []string{common.GoOra, common.Godror} {
		for _, positional := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", driver, positional), func(t *testing.T) {
				query := schemaSQL{text: "SELECT :b, :a, :b FROM dual", args: []any{sql.Named("a", 1), sql.Named("b", 2)}, positional: positional}
				c := &Check{driver: driver}
				actual, err := c.schemaQueryArguments(query)
				require.NoError(t, err)
				if driver == common.GoOra && positional {
					require.Equal(t, []any{2, 1, 2}, actual)
				} else {
					require.Equal(t, query.args, actual)
				}
			})
		}
	}
}

func TestSchemaQueryArgumentsMatchGeneratedQueries(t *testing.T) {
	for _, driver := range []string{common.GoOra, common.Godror} {
		for _, count := range []int{1, 100, 101} {
			t.Run(fmt.Sprintf("%s/%d", driver, count), func(t *testing.T) {
				c, db, mock, cleanup := newSchemaCheck(t)
				defer cleanup()
				c.driver = driver
				keys := schemaHydrationKeys(count, "Quoted'Owner", 3)
				selected := make(map[tableKey]struct{}, len(keys))
				for _, key := range keys {
					selected[key] = struct{}{}
				}
				calls := 0
				c.schemaQueryer = schemaQueryFunc(func(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
					calls++
					require.NotContains(t, query, "Quoted'Owner")
					occurrences := regexp.MustCompile(`:[A-Za-z0-9_]+`).FindAllString(query, -1)
					if driver == common.GoOra {
						require.Len(t, args, len(occurrences))
						values := make(map[string]any)
						for i, placeholder := range occurrences {
							_, named := args[i].(sql.NamedArg)
							require.False(t, named)
							if previous, ok := values[placeholder]; ok {
								require.Equal(t, previous, args[i])
							}
							values[placeholder] = args[i]
						}
						require.Contains(t, args, "Quoted'Owner")
					} else {
						for _, arg := range args {
							_, named := arg.(sql.NamedArg)
							require.True(t, named)
						}
					}
					return db.QueryxContext(ctx, query, args...)
				})
				for start := 0; start < count; start += 100 {
					mock.ExpectQuery("WITH ranked_columns").WillReturnRows(emptyTablesRows())
				}
				require.NoError(t, c.hydrateTablePages(context.Background(), keys, 50, func([]schemaRowDB) error { return nil }))
				mock.ExpectQuery("FROM cdb_constraints").WillReturnRows(emptyConstraintRows())
				c.tableConstraintsForPage(context.Background(), selected, selected)
				columns := make(map[columnKey]struct{}, len(selected))
				for key := range selected {
					columns[columnKey{tableKey: key, column: "ID"}] = struct{}{}
				}
				filters := columnFilterChunks(columns, relationColumnNames{conID: "con_id", owner: "owner", relation: "table_name"}, "column_name")
				for range filters {
					mock.ExpectQuery("FROM cdb_col_comments").WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
				}
				c.loadSchemaColumnDetails(context.Background(), filters, nil, func(columnKey) *tableDetails { t.Fatal("unexpected row"); return nil })
				require.Equal(t, (count+99)/100+1+len(filters), calls)
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

func TestSchemaPrefetchPreservesConnectionOptions(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, protocol := range []string{"TCP", "TCPS"} {
			t.Run(fmt.Sprintf("%v/%s", enabled, protocol), func(t *testing.T) {
				c := &Check{config: &config.CheckConfig{}}
				c.config.Schemas.Enabled = enabled
				c.config.Server, c.config.Port, c.config.ServiceName = "localhost", 1521, "PDB"
				c.config.Username, c.config.Password = "test", "test password"
				c.config.QueryTimeout, c.config.Protocol = 42, protocol
				c.config.Wallet = "/test wallet"
				u, err := url.Parse(buildGoOraURL(c))
				require.NoError(t, err)
				require.Equal(t, "localhost:1521", u.Host)
				require.Equal(t, "/PDB", u.Path)
				require.Equal(t, "test", u.User.Username())
				password, _ := u.User.Password()
				require.Equal(t, "test password", password)
				require.Equal(t, "42", u.Query().Get("TIMEOUT"))
				if enabled {
					require.Equal(t, "100", u.Query().Get("PREFETCH_ROWS"))
				} else {
					require.Empty(t, u.Query().Get("PREFETCH_ROWS"))
				}
				if protocol == "TCPS" {
					require.Equal(t, "TRUE", u.Query().Get("SSL"))
					require.Equal(t, "/test wallet", u.Query().Get("WALLET"))
				} else {
					require.Empty(t, u.Query().Get("SSL"))
					require.Empty(t, u.Query().Get("WALLET"))
				}
			})
		}
	}
}

func TestSchemaFiltersFactorSharedContainer(t *testing.T) {
	names := relationColumnNames{conID: "c.con_id", owner: "c.owner", relation: "c.table_name"}
	for _, sameContainer := range []bool{true, false} {
		t.Run(fmt.Sprintf("sameContainer=%t", sameContainer), func(t *testing.T) {
			otherID := int64(3)
			if !sameContainer {
				otherID = 4
			}
			first := tableKey{conID: 3, owner: "APP", table: "T"}
			second := tableKey{conID: otherID, owner: "OTHER", table: "T"}
			for _, filters := range [][]schemaSQL{
				relationFilterChunks(map[tableKey]struct{}{first: {}, second: {}}, names),
				columnFilterChunks(map[columnKey]struct{}{
					{tableKey: first, column: "ID"}: {}, {tableKey: second, column: "ID"}: {},
				}, names, "c.column_name"),
			} {
				require.Len(t, filters, 1)
				filter := filters[0]
				if sameContainer {
					require.True(t, strings.HasPrefix(filter.text, "(c.con_id = :pagecon AND ((c.owner = :"))
					require.Equal(t, 1, strings.Count(filter.text, "c.con_id ="))
					require.Equal(t, sql.Named("pagecon", int64(3)), filter.args[0])
				} else {
					require.NotContains(t, filter.text, ":pagecon")
					require.Equal(t, 2, strings.Count(filter.text, "c.con_id ="))
					require.Contains(t, filter.text, " OR (c.con_id = :")
				}
				require.NotContains(t, filter.text, "APP")
				require.NotContains(t, filter.text, "OTHER")
			}
		})
	}
}

func TestSchemaJoinedFiltersReuseBindings(t *testing.T) {
	primary := relationColumnNames{conID: "c.con_id", owner: "c.owner", relation: "c.constraint_name"}
	joined := relationColumnNames{conID: "cc.con_id", owner: "cc.owner", relation: "cc.constraint_name"}
	for _, otherID := range []int64{3, 4} {
		t.Run(strconv.FormatInt(otherID, 10), func(t *testing.T) {
			allowed := map[tableKey]struct{}{
				{conID: 3, owner: "APP", table: "PK_O'RDER"}:        {},
				{conID: otherID, owner: "OTHER", table: "PK_OTHER"}: {},
			}
			first := relationFilterChunks(allowed, primary)
			second := relationFilterChunks(allowed, joined)
			both := relationFilterChunks(allowed, primary, joined)
			require.Len(t, both, 1)
			require.Equal(t, first[0].text+" AND "+second[0].text, both[0].text)
			require.Equal(t, first[0].args, both[0].args)
			require.NotContains(t, both[0].text, "PK_O'RDER")
			seen := make(map[string]bool)
			for _, arg := range both[0].args {
				name := arg.(sql.NamedArg).Name
				require.False(t, seen[name], "duplicate bind %s", name)
				seen[name] = true
			}
		})
	}
}

func TestSchemaMultiOwnerFilterShapesStayStable(t *testing.T) {
	columns := relationColumnNames{conID: "t.con_id", owner: "t.owner", relation: "t.table_name"}
	var previous string
	for _, count := range []int{1, 50} {
		allowed := make(map[tableKey]struct{})
		for _, owner := range []string{"APP", "OTHER"} {
			for i := range count {
				allowed[tableKey{conID: 3, owner: owner, table: fmt.Sprintf("T%d", i)}] = struct{}{}
			}
		}
		filters := relationFilterChunks(allowed, columns)
		require.Len(t, filters, 1)
		if previous != "" {
			require.Equal(t, previous, filters[0].text)
		}
		previous = filters[0].text
		require.Len(t, filters[0].args, 1+2*(1+schemaRelationPageSize))
	}
}

func TestSchemaIndexQueriesPreserveJoinIndexColumns(t *testing.T) {
	require.Empty(t, schemaIndexQueries(nil))
	for _, otherID := range []int64{3, 4} {
		t.Run(strconv.FormatInt(otherID, 10), func(t *testing.T) {
			allowed := map[tableKey]struct{}{
				{conID: 3, owner: "APP", table: "FACT"}:      {},
				{conID: otherID, owner: "OTHER", table: "T"}: {},
			}
			primary := relationFilterChunks(allowed, relationColumnNames{conID: "i.con_id", owner: "i.table_owner", relation: "i.table_name"})
			columns := relationFilterChunks(allowed, relationColumnNames{conID: "ic.con_id", owner: "ic.table_owner", relation: "ic.table_name"})
			queries := schemaIndexQueries(allowed)
			require.Len(t, queries, 1)
			query := queries[0]
			require.True(t, strings.HasPrefix(query.text, "SELECT con_id, table_owner, table_name, index_owner, index_name, uniqueness, index_type, column_name\nFROM ("))
			require.NotContains(t, query.text, "/*")
			branches := strings.Split(query.text, "UNION ALL")
			require.Len(t, branches, 2)
			require.Contains(t, branches[0], "WHERE NVL(i.join_index, 'NO') <> 'YES' AND "+primary[0].text+" AND "+columns[0].text)
			require.Contains(t, branches[1], "WHERE i.join_index = 'YES' AND "+primary[0].text)
			require.NotContains(t, branches[1], "ic.table_owner")
			require.NotContains(t, branches[1], "ic.table_name")
			for _, branch := range branches {
				require.Contains(t, branch, "ON ic.con_id = i.con_id AND ic.index_owner = i.owner AND ic.index_name = i.index_name")
			}
			require.True(t, strings.HasSuffix(query.text, "ORDER BY con_id, table_owner, table_name, index_owner, index_name, column_position"))
			require.Equal(t, primary[0].args, columns[0].args)
			require.Equal(t, primary[0].args, query.args)
			bindings := make(map[string]bool)
			for _, arg := range query.args {
				name := arg.(sql.NamedArg).Name
				require.False(t, bindings[name], "duplicate bind %s", name)
				bindings[name] = true
			}
			for _, placeholder := range regexp.MustCompile(`:[A-Za-z][A-Za-z0-9]*`).FindAllString(query.text, -1) {
				require.True(t, bindings[strings.TrimPrefix(placeholder, ":")], "unbound placeholder %s", placeholder)
			}
		})
	}
}

func TestSchemaIndexQueriesKeepChunkBindingsAligned(t *testing.T) {
	allowed := make(map[tableKey]struct{})
	for i := 0; i <= maxSchemaRelationsPerQuery; i++ {
		allowed[tableKey{conID: int64(3 + i%2), owner: fmt.Sprintf("APP%d", i%3), table: fmt.Sprintf("T%d", i)}] = struct{}{}
	}
	queries := schemaIndexQueries(allowed)
	primary := relationFilterChunks(allowed, relationColumnNames{conID: "i.con_id", owner: "i.table_owner", relation: "i.table_name"})
	columns := relationFilterChunks(allowed, relationColumnNames{conID: "ic.con_id", owner: "ic.table_owner", relation: "ic.table_name"})
	require.Len(t, queries, 2)
	for i, query := range queries {
		require.Equal(t, primary[i].args, columns[i].args)
		require.Equal(t, primary[i].args, query.args)
		require.Contains(t, query.text, primary[i].text)
		require.Contains(t, query.text, columns[i].text)
	}
}

func TestSchemaBindingsColumnListSizesAreBounded(t *testing.T) {
	for _, tc := range []struct{ count, slots int }{
		{1, 50}, {50, 50}, {51, 100}, {101, 200}, {801, 1000}, {1000, 1000},
	} {
		t.Run(strconv.Itoa(tc.count), func(t *testing.T) {
			names := make([]string, tc.count)
			for i := range names {
				names[i] = fmt.Sprintf("C%d", i)
			}
			var query schemaSQL
			query.text = query.stringList("column", names, schemaColumnBindSlots)
			require.Len(t, query.args, tc.slots)
			require.Equal(t, tc.slots, strings.Count(query.text, ":column"))
			for i, arg := range query.args {
				value := arg.(sql.NamedArg).Value.(sql.NullString)
				require.Equal(t, i < tc.count, value.Valid)
				if value.Valid {
					require.Equal(t, names[i], value.String)
				}
			}
		})
	}
}

func TestSchemaBindingsEmptySelectionsDoNotQuery(t *testing.T) {
	require.Empty(t, relationFilterChunks(nil, relationColumnNames{}))
	require.Empty(t, columnFilterChunks(nil, relationColumnNames{}, "column_name"))
	require.Empty(t, ownerListChunks(nil))
}

func TestSchemaBindingsBoundPaddingAcrossGroups(t *testing.T) {
	relations := make(map[tableKey]struct{})
	columns := make(map[columnKey]struct{})
	for i := range maxSchemaRelationsPerQuery + 1 {
		key := tableKey{conID: 3, owner: fmt.Sprintf("OWNER%d", i), table: "T"}
		relations[key] = struct{}{}
		columns[columnKey{tableKey: key, column: "ID"}] = struct{}{}
	}
	names := relationColumnNames{conID: "con_id", owner: "owner", relation: "table_name"}
	for _, filters := range [][]schemaSQL{relationFilterChunks(relations, names), columnFilterChunks(columns, names, "column_name")} {
		require.Len(t, filters, 2)
		for _, filter := range filters {
			require.LessOrEqual(t, len(filter.args), 5*maxSchemaRelationsPerQuery)
		}
	}
}

func TestSchemaBindingsHydrationSuppliesEveryNamedArgument(t *testing.T) {
	c, _, _, cleanup := newSchemaCheck(t)
	defer cleanup()
	keys := []tableKey{
		{conID: 3, owner: "APP", table: "O'RDER"},
		{conID: 3, owner: "OTHER", table: "TABLE"},
	}
	for _, view := range []bool{false, true} {
		c.schemaQueryer = schemaQueryFunc(func(_ context.Context, query string, args ...any) (*sqlx.Rows, error) {
			require.NotContains(t, query, "O'RDER")
			require.NotContains(t, query, "/*RELATIONS*/")
			provided := make(map[string]bool)
			for _, arg := range args {
				named := arg.(sql.NamedArg)
				require.False(t, provided[named.Name], "duplicate bind %s", named.Name)
				provided[named.Name] = true
			}
			for _, match := range regexp.MustCompile(`:[a-zA-Z][a-zA-Z0-9_]*`).FindAllString(query, -1) {
				require.True(t, provided[match[1:]], "missing bind %s", match)
			}
			for name := range provided {
				require.Contains(t, query, ":"+name)
			}
			return nil, context.Canceled
		})
		c.schemaQueryError = nil
		if view {
			_, err := c.viewPageRows(context.Background(), keys, 17)
			require.ErrorIs(t, err, context.Canceled)
		} else {
			_, err := c.tablePageRows(context.Background(), keys, 17)
			require.ErrorIs(t, err, context.Canceled)
		}
	}
}

func TestSchemaColumnDefaultQueriesKeepSingleOwnerUnchanged(t *testing.T) {
	for _, count := range []int{1, 2, 100} {
		allowed := make(map[columnKey]struct{})
		for table := range count {
			for _, name := range []string{"ID", "VALUE"} {
				allowed[columnKey{tableKey: tableKey{conID: 3, owner: "APP", table: fmt.Sprintf("T%d", table)}, column: name}] = struct{}{}
			}
		}
		filters := columnFilterChunks(allowed, relationColumnNames{conID: "c.con_id", owner: "c.owner", relation: "c.table_name"}, "c.column_name")
		queries := schemaColumnDefaultQueries(allowed)
		require.Len(t, queries, len(filters))
		for i, filter := range filters {
			template := strings.Replace(columnDefaultsQuery, "/*DEFAULT_COL*/", "c.data_default_vc", 1)
			require.Equal(t, strings.Replace(template, "/*RELATIONS*/", filter.text, 1), queries[i].text)
			require.Equal(t, filter.args, queries[i].args)
		}
	}
	require.Empty(t, schemaColumnDefaultQueries(nil))
}

func TestSchemaColumnDefaultQueriesKeepSQLStableAcrossValues(t *testing.T) {
	build := func(prefix string, container int64) schemaSQL {
		selected := make(map[columnKey]struct{})
		for owner := range 2 {
			for table := range 2 {
				selected[columnKey{tableKey: tableKey{conID: container, owner: fmt.Sprintf("%s%d", prefix, owner), table: fmt.Sprintf("%s%d", prefix, table)}, column: prefix + "COL"}] = struct{}{}
			}
		}
		return schemaColumnDefaultQueries(selected)[0]
	}
	first, second := build("A'", 3), build("B:owner1_", 4)
	require.Equal(t, first.text, second.text)
	require.NotEqual(t, first.args, second.args)
	nulls := 0
	for _, raw := range first.args {
		if value, ok := raw.(sql.NamedArg).Value.(sql.NullString); ok && !value.Valid {
			nulls++
		}
	}
	require.Positive(t, nulls)
}

func TestSchemaColumnDefaultQueriesBoundOwnerBranches(t *testing.T) {
	for _, test := range []struct{ owners, tables, columns int }{{2, 40, 10}, {3, 33, 10}, {10, 10, 10}, {100, 1, 10}, {1001, 1, 1}} {
		t.Run(fmt.Sprintf("%d_%d_%d", test.owners, test.tables, test.columns), func(t *testing.T) {
			allowed := make(map[columnKey]struct{})
			for owner := range test.owners {
				for table := range test.tables {
					for column := range test.columns {
						allowed[columnKey{tableKey: tableKey{conID: 3, owner: fmt.Sprintf("O%04d", owner), table: fmt.Sprintf("T%04d", table)}, column: fmt.Sprintf("C%04d", column)}] = struct{}{}
					}
				}
			}
			queries := schemaColumnDefaultQueries(allowed)
			branches := 0
			for _, query := range queries {
				require.LessOrEqual(t, len(query.args), 5*maxSchemaRelationsPerQuery)
				branches += strings.Count(query.text, "SELECT c.con_id")
				bindings := make(map[string]any)
				for _, raw := range query.args {
					arg := raw.(sql.NamedArg)
					_, duplicate := bindings[arg.Name]
					require.False(t, duplicate, arg.Name)
					bindings[arg.Name] = arg.Value
				}
				used := make(map[string]struct{})
				for _, match := range regexp.MustCompile(`:([A-Za-z0-9_]+)`).FindAllStringSubmatch(query.text, -1) {
					require.Contains(t, bindings, match[1])
					used[match[1]] = struct{}{}
				}
				require.Len(t, used, len(bindings))
				require.NotContains(t, query.text, "O0000")
				require.NotContains(t, query.text, "T0000")
				require.Contains(t, query.text, "c.default_length > 0")
			}
			require.Equal(t, test.owners, branches)
			if test.owners == 100 {
				require.Len(t, queries, 2)
				require.Len(t, queries[0].args, 94*53)
				require.Len(t, queries[1].args, 6*53)
			}
			if test.owners == 1001 {
				require.Len(t, queries, 12)
				require.NotContains(t, queries[len(queries)-1].text, "UNION ALL")
			}
		})
	}
}

func TestSchemaColumnDefaultQueriesKeepExactHeterogeneousBranches(t *testing.T) {
	groups := []map[columnKey]struct{}{
		{{tableKey: tableKey{conID: 3, owner: "O':owner0_", table: "A"}, column: "C:10"}: {},
			{tableKey: tableKey{conID: 3, owner: "O':owner0_", table: "B"}, column: "OTHER"}: {}},
		{{tableKey: tableKey{conID: 4, owner: "O':owner0_", table: "A"}, column: "C:10"}: {}},
	}
	allowed := make(map[columnKey]struct{})
	for _, group := range groups {
		for key := range group {
			allowed[key] = struct{}{}
		}
	}
	queries := schemaColumnDefaultQueries(allowed)
	require.Len(t, queries, 1)
	var expectedArgs []any
	var expectedBranches []string
	for i, group := range groups {
		filter := columnFilterChunks(group, relationColumnNames{conID: "c.con_id", owner: "c.owner", relation: "c.table_name"}, "c.column_name")[0]
		prefix := fmt.Sprintf("owner%d_", i)
		for _, raw := range filter.args {
			arg := raw.(sql.NamedArg)
			expectedArgs = append(expectedArgs, sql.Named(prefix+arg.Name, arg.Value))
		}
		template := strings.Replace(columnDefaultsQuery, "/*DEFAULT_COL*/", "c.data_default_vc", 1)
		expectedBranches = append(expectedBranches, strings.Replace(template, "/*RELATIONS*/", strings.ReplaceAll(filter.text, ":", ":"+prefix), 1))
	}
	require.Equal(t, expectedArgs, queries[0].args)
	require.Equal(t, strings.Join(expectedBranches, " UNION ALL "), queries[0].text)
	require.NotContains(t, queries[0].text, "O':owner0_")
	require.NotContains(t, queries[0].text, "C:10")
	require.Contains(t, queries[0].text, ":owner0_col0name1,")
	require.Contains(t, queries[0].text, ":owner0_col0name10,")
}

func TestSchemaColumnDefaultQueriesPreserveOriginalColumnChunks(t *testing.T) {
	allowed := make(map[columnKey]struct{})
	for i := range 1100 {
		allowed[columnKey{tableKey: tableKey{conID: 3, owner: "A", table: "WIDE"}, column: fmt.Sprintf("C%04d", i)}] = struct{}{}
	}
	allowed[columnKey{tableKey: tableKey{conID: 3, owner: "B", table: "T"}, column: "ID"}] = struct{}{}
	queries := schemaColumnDefaultQueries(allowed)
	require.Len(t, queries, 2)
	require.NotContains(t, queries[0].text, "UNION ALL")
	require.Equal(t, 1, strings.Count(queries[1].text, "UNION ALL"))
	seen := make(map[string]int)
	for _, query := range queries {
		for _, raw := range query.args {
			arg := raw.(sql.NamedArg)
			if value, ok := arg.Value.(sql.NullString); ok && value.Valid && strings.HasPrefix(value.String, "C") {
				seen[value.String]++
			}
		}
	}
	require.Len(t, seen, 1100)
	for name, count := range seen {
		require.Equal(t, 1, count, name)
	}
	require.Contains(t, queries[0].text, ":col0name100,")
}

func TestSchemaColumnDefaultQueriesPreserveOptionalFailurePolicy(t *testing.T) {
	for _, test := range []struct {
		name       string
		queryError error
		rowError   error
		empty      bool
	}{
		{name: "values_and_null"},
		{name: "missing_rows", empty: true},
		{name: "partial_rows", rowError: errors.New("row failed")},
		{name: "unavailable", queryError: errors.New("ORA-00904: unavailable")},
		{name: "deadline", queryError: context.DeadlineExceeded},
		{name: "cancelled", queryError: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, db, mock, cleanup := newSchemaCheck(t)
			defer cleanup()
			first := tableKey{conID: 3, owner: "A", table: "T"}
			second := tableKey{conID: 3, owner: "B", table: "T"}
			allowed := map[tableKey]struct{}{first: {}, second: {}}
			columns := map[columnKey]struct{}{{tableKey: first, column: "ID"}: {}, {tableKey: second, column: "ID"}: {}}
			query := schemaColumnDefaultQueries(columns)[0]
			c.schemaQueryer = schemaQueryFunc(func(ctx context.Context, sql string, args ...any) (*sqlx.Rows, error) {
				if strings.Contains(sql, "c.data_default_vc AS data_default") {
					return db.QueryxContext(ctx, sql, args...)
				}
				return nil, errors.New("ORA-00942: optional metadata unavailable")
			})
			expectation := mock.ExpectQuery(regexp.QuoteMeta(query.text))
			if test.queryError != nil {
				expectation.WillReturnError(test.queryError)
			} else {
				rows := sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "DATA_DEFAULT"})
				if !test.empty {
					rows.AddRow(3, "A", "T", "ID", "42").AddRow(3, "B", "T", "ID", nil)
					if test.rowError != nil {
						rows.RowError(1, test.rowError)
					}
				}
				expectation.WillReturnRows(rows)
			}
			details := c.tableDetailsForPage(context.Background(), allowed, columns)
			require.NotNil(t, details[first])
			require.NotNil(t, details[second])
			require.True(t, details[first].PropertiesIncomplete)
			require.True(t, details[second].PropertiesIncomplete)
			if test.queryError == nil && !test.empty {
				require.Equal(t, "42", details[first].ColumnDefaults["ID"])
			} else {
				require.Nil(t, details[first].ColumnDefaults)
			}
			require.Nil(t, details[second].ColumnDefaults)
			if errors.Is(test.queryError, context.DeadlineExceeded) || errors.Is(test.queryError, context.Canceled) {
				require.ErrorIs(t, c.schemaContextError(context.Background()), test.queryError)
			} else {
				require.NoError(t, c.schemaContextError(context.Background()))
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSchemaColumnDefaultQueriesLeaveLegacyPathUnchanged(t *testing.T) {
	for _, version := range []string{"12.1.0.2.0", "19.0.0.0.0", "21.3.0.0.0"} {
		t.Run(version, func(t *testing.T) {
			c, _, _, cleanup := newSchemaCheck(t)
			defer cleanup()
			c.dbVersion = version
			calls := 0
			c.schemaQueryer = schemaQueryFunc(func(_ context.Context, query string, _ ...any) (*sqlx.Rows, error) {
				if strings.Contains(query, "c.data_default_vc AS data_default") {
					calls++
				}
				return nil, errors.New("ORA-00942: optional metadata unavailable")
			})
			key := tableKey{conID: 3, owner: "A", table: "T"}
			c.tableDetailsForPage(context.Background(), map[tableKey]struct{}{key: {}}, map[columnKey]struct{}{{tableKey: key, column: "ID"}: {}})
			require.Zero(t, calls)
			require.Equal(t, "c.data_default", c.defaultValueColumn())
		})
	}
}

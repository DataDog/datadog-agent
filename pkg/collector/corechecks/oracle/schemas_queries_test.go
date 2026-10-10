// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

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

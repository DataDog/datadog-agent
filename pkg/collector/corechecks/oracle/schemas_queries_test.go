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

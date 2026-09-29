// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"database/sql"
	"strconv"
	"strings"
)

type schemaSQL struct {
	text string
	args []any
}

func (q *schemaSQL) bind(name string, value any) string {
	q.args = append(q.args, sql.Named(name, value))
	return ":" + name
}

func (q *schemaSQL) stringList(prefix string, values []string, size int) string {
	// Reserve extra placeholders so Oracle can reuse the cached query as the list grows.
	// For example, IN (:val0, :val1, :val2, :val3) receives ["A", "B", NULL, NULL]
	// for two values. A third value replaces the first NULL without changing the SQL text.
	for size < len(values) {
		size *= 2
	}
	if size > maxSchemaRelationsPerQuery {
		size = maxSchemaRelationsPerQuery
	}
	placeholders := make([]string, size)
	for i := range placeholders {
		var value sql.NullString
		if i < len(values) {
			value = sql.NullString{String: values[i], Valid: true}
		}
		placeholders[i] = q.bind(prefix+strconv.Itoa(i), value)
	}
	return strings.Join(placeholders, ", ")
}

func regexSQLClauses(column string, include, exclude []string) schemaSQL {
	var q schemaSQL
	var b strings.Builder
	for i, pattern := range exclude {
		b.WriteString(" AND NOT REGEXP_LIKE(" + column + ", " + q.bind("exclude"+strconv.Itoa(i), pattern) + ", 'i')")
	}
	if len(include) > 0 {
		parts := make([]string, len(include))
		for i, pattern := range include {
			parts[i] = "REGEXP_LIKE(" + column + ", " + q.bind("include"+strconv.Itoa(i), pattern) + ", 'i')"
		}
		b.WriteString(" AND (" + strings.Join(parts, " OR ") + ")")
	}
	q.text = b.String()
	return q
}

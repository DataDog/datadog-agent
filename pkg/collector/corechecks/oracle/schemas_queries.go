// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
)

type schemaSQL struct {
	text       string
	args       []any
	positional bool
}

func (q *schemaSQL) bind(name string, value any) string {
	q.args = append(q.args, sql.Named(name, value))
	return ":" + name
}

func (q *schemaSQL) stringList(prefix string, values []string, size int) string {
	// NULL padding keeps SQL text stable so Oracle can reuse cached statements.
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

func (c *Check) schemaQueryArguments(query schemaSQL) ([]any, error) {
	if !query.positional || c.driver != common.GoOra {
		return query.args, nil
	}
	// go-ora repeatedly searches named arguments; positional binds avoid that cost.
	return schemaBindOrderArguments(query)
}

func schemaBindOrderArguments(statement schemaSQL) ([]any, error) {
	values := make(map[string]any, len(statement.args))
	for _, raw := range statement.args {
		named, ok := raw.(sql.NamedArg)
		if !ok || named.Name == "" {
			return nil, errors.New("expected named arguments")
		}
		name := strings.ToLower(named.Name)
		if _, exists := values[name]; exists {
			return nil, errors.New("duplicate named argument")
		}
		values[name] = named.Value
	}
	var ordered []any
	used := make(map[string]bool)
	text := statement.text
	for i := 0; i < len(text); {
		if text[i] == '\'' || text[i] == '"' {
			if i > 0 && (text[i-1] == 'q' || text[i-1] == 'Q') && text[i] == '\'' {
				return nil, errors.New("alternative quoted literal is unsupported in positional schema SQL")
			}
			quote := text[i]
			i++
			closed := false
			for i < len(text) {
				if text[i] != quote {
					i++
					continue
				}
				i++
				if i < len(text) && text[i] == quote {
					i++
					continue
				}
				closed = true
				break
			}
			if !closed {
				return nil, errors.New("unterminated quote")
			}
			continue
		}
		if strings.HasPrefix(text[i:], "/*") {
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				return nil, errors.New("unterminated comment")
			}
			i += end + 4
			continue
		}
		if strings.HasPrefix(text[i:], "--") {
			end := strings.IndexByte(text[i:], '\n')
			if end < 0 {
				break
			}
			i += end + 1
			continue
		}
		if text[i] != ':' {
			i++
			continue
		}
		start := i + 1
		end := start
		for end < len(text) && ((text[end] >= 'a' && text[end] <= 'z') || (text[end] >= 'A' && text[end] <= 'Z') ||
			(text[end] >= '0' && text[end] <= '9') || text[end] == '_') {
			end++
		}
		if end == start {
			return nil, errors.New("unsupported colon syntax")
		}
		name := strings.ToLower(text[start:end])
		value, ok := values[name]
		if !ok {
			return nil, fmt.Errorf("missing argument %s", name)
		}
		ordered = append(ordered, value)
		used[name] = true
		i = end
	}
	if len(used) != len(values) {
		return nil, errors.New("unused argument")
	}
	return ordered, nil
}

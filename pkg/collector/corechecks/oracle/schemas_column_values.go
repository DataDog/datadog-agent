// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const schemaCurrentContainerID = "TO_NUMBER(SYS_CONTEXT('USERENV', 'CON_ID'))"

const schemaSessionContainerQuery = `SELECT TO_NUMBER(SYS_CONTEXT('USERENV', 'CON_ID')),
SYS_CONTEXT('USERENV', 'CON_NAME') FROM dual`

// Before 23ai, CDB_TAB_COLS omits the LONG default. Read it inside its PDB instead.
const localColumnValuesQuery = `SELECT TO_NUMBER(SYS_CONTEXT('USERENV', 'CON_ID')),
c.owner, c.table_name, c.column_name, c.data_default
FROM dba_tab_cols c WHERE /*RELATIONS*/`

type schemaContainerSession interface {
	schemaQueryer
	QueryRowxContext(context.Context, string, ...any) *sqlx.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	Raw(func(any) error) error
}

func (c *Check) enrichSchemaColumnValues(ctx context.Context, details map[tableKey]*tableDetails, columns map[columnKey]struct{}) {
	expressions := make(map[columnKey][]*indexKeyPart)
	selected := make(map[columnKey]struct{})
	local := c.defaultValueColumn() == "c.data_default"
	if local {
		for key := range columns {
			selected[key] = struct{}{}
		}
	}
	for table, detail := range details {
		for _, index := range detail.Indexes {
			for i := range index.Columns {
				part := &index.Columns[i]
				if strings.HasPrefix(part.Column, "SYS_NC") {
					key := columnKey{tableKey: table, column: part.Column}
					selected[key] = struct{}{}
					expressions[key] = append(expressions[key], part)
				}
			}
		}
	}
	scan := func(rows *sqlx.Rows) error {
		var key columnKey
		var value sql.NullString
		if err := rows.Scan(&key.conID, &key.owner, &key.table, &key.column, &value); err != nil {
			return err
		}
		if !value.Valid {
			return nil
		}
		text := truncateLongValue(value.String)
		if _, ok := columns[key]; local && ok {
			if details[key.tableKey] == nil {
				details[key.tableKey] = &tableDetails{}
			}
			detail := details[key.tableKey]
			if detail.ColumnDefaults == nil {
				detail.ColumnDefaults = make(map[string]string)
			}
			detail.ColumnDefaults[key.column] = text
		}
		for _, part := range expressions[key] {
			*part = indexKeyPart{Expression: text}
		}
		return nil
	}
	if !local {
		filters := columnFilterChunks(selected, relationColumnNames{conID: "c.con_id", owner: "c.owner", relation: "c.table_name"}, "c.column_name")
		query := strings.Replace(columnDefaultsQuery, "/*DEFAULT_COL*/", "c.data_default_vc", 1)
		c.queryDetailFilters(ctx, "index expressions", query, filters, scan)
		return
	}
	byContainer := make(map[int64]map[columnKey]struct{})
	for key := range selected {
		if byContainer[key.conID] == nil {
			byContainer[key.conID] = make(map[columnKey]struct{})
		}
		byContainer[key.conID][key] = struct{}{}
	}
	ids := make([]int64, 0, len(byContainer))
	for id := range byContainer {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		err := c.withSchemaContainer(ctx, id, func() error {
			filters := columnFilterChunks(byContainer[id], relationColumnNames{conID: schemaCurrentContainerID, owner: "c.owner", relation: "c.table_name"}, "c.column_name")
			for _, filter := range filters {
				query := strings.Replace(localColumnValuesQuery, "/*RELATIONS*/", filter.text, 1)
				if err := c.queryMetadata(ctx, query, scan, filter.args...); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			log.Warnf("%s failed to collect column defaults and index expressions in container %d: %v", c.logPrompt, id, err)
		}
	}
}

func (c *Check) withSchemaContainer(ctx context.Context, id int64, collect func() error) (err error) {
	defer func() {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			c.schemaQueryError = err
		}
	}()
	if err := c.schemaContextError(ctx); err != nil {
		return err
	}
	conn, ok := c.schemaQueryer.(schemaContainerSession)
	if !ok {
		if c.schemaQueryer != nil {
			return errors.New("schema query connection cannot switch containers")
		}
		if c.db == nil {
			return errors.New("schema connection pool is unavailable")
		}
		acquireCtx, cancel := context.WithTimeout(ctx, c.config.QueryTimeoutDuration())
		defer cancel()
		acquired, err := c.db.Connx(acquireCtx)
		if err != nil {
			return err
		}
		cancel()
		defer acquired.Close()
		conn = acquired
	}
	previous := c.schemaQueryer
	c.schemaQueryer = conn
	defer func() { c.schemaQueryer = previous }()
	queryCtx, cancel := context.WithTimeout(ctx, c.config.Schemas.MaxQueryDurationDuration())
	defer cancel()
	var originalID int64
	var originalName string
	if err := conn.QueryRowxContext(queryCtx, schemaSessionContainerQuery).Scan(&originalID, &originalName); err != nil {
		return err
	}
	if originalID == id {
		return collect()
	}
	var target string
	if err := conn.QueryRowxContext(queryCtx, "SELECT name FROM v$containers WHERE con_id = :con_id", sql.Named("con_id", id)).Scan(&target); err != nil {
		return err
	}
	defer func() {
		// Cancellation must not leave a PDB session in the shared metrics pool.
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), c.config.QueryTimeoutDuration())
		defer restoreCancel()
		_, restoreErr := conn.ExecContext(restoreCtx, schemaSwitchContainerSQL(originalName))
		if restoreErr == nil {
			var restoredID int64
			var restoredName string
			restoreErr = conn.QueryRowxContext(restoreCtx, schemaSessionContainerQuery).Scan(&restoredID, &restoredName)
			if restoreErr == nil && restoredID != originalID {
				restoreErr = fmt.Errorf("expected container %d, got %d", originalID, restoredID)
			}
		}
		if restoreErr != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			c.schemaQueryError = fmt.Errorf("restore schema connection container: %w", restoreErr)
			err = errors.Join(err, c.schemaQueryError)
		}
	}()
	if _, err := conn.ExecContext(queryCtx, schemaSwitchContainerSQL(target)); err != nil {
		return err
	}
	var actualID int64
	var actualName string
	if err := conn.QueryRowxContext(queryCtx, schemaSessionContainerQuery).Scan(&actualID, &actualName); err != nil {
		return err
	}
	if actualID != id {
		return fmt.Errorf("schema container changed: expected %d, got %d", id, actualID)
	}
	return collect()
}

func schemaSwitchContainerSQL(name string) string {
	return `ALTER SESSION SET CONTAINER = "` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

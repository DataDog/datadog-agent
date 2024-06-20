// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && test

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cilium/ebpf"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// DummyTable defines a table schema, so that `bun` module can generate queries for it.
type DummyTable struct {
	bun.BaseModel `bun:"table:dummy,alias:d"`

	ID  int64 `bun:",pk,autoincrement"`
	Foo string
}

var dummyModel = &DummyTable{ID: 1, Foo: "bar"}

// ConnectionOptions represents the different configurable settings for a connection to a Postgres DB.
type ConnectionOptions struct {
	ServerAddress string
	Username      string
	Password      string
	DBName        string
	EnableTLS     bool
}

func buildDSN(opts ConnectionOptions) string {
	sslMode := "disable"
	if opts.EnableTLS {
		sslMode = "allow"
	}
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s",
		optionOrDefault(opts.Username, "", "admin"),
		optionOrDefault(opts.Password, "", "password"),
		optionOrDefault(opts.ServerAddress, "", "localhost:5432"),
		optionOrDefault(opts.DBName, "", "testdb"),
		sslMode,
	)
}

// PGClient is a simple wrapper around the `bun` module to interact with a Postgres DB.
type PGClient struct {
	db *bun.DB
}

// NewPGClient creates a new Postgres client for testing purposes.
func NewPGClient(opts ConnectionOptions) *PGClient {
	return &PGClient{
		db: bun.NewDB(sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(buildDSN(opts)))), pgdialect.New()),
	}
}

// DB returns the underlying `bun` DB object.
func (c *PGClient) DB() *bun.DB {
	return c.db
}

// Ping the test DB to check if it is reachable.
func (c *PGClient) Ping() error {
	return c.db.Ping()
}

// Close closes the connection to the test DB.
func (c *PGClient) Close() error {
	return c.db.Close()
}

// RunAlterQuery runs ALTER query on the test DB.
func (c *PGClient) RunAlterQuery() error {
	return runTimedQuery(c.db.NewAddColumn().Model((*DummyTable)(nil)).ColumnExpr("new_column BOOL").Exec)
}

// RunCreateQuery creates a new table.
func (c *PGClient) RunCreateQuery() error {
	return runTimedQuery(c.db.NewCreateTable().Model((*DummyTable)(nil)).Exec)
}

// RunDeleteQuery run a deletion query on the test DB.
func (c *PGClient) RunDeleteQuery() error {
	return runTimedQuery(c.db.NewDelete().Model(dummyModel).WherePK().Exec)
}

// RunDropQuery drops a table.
func (c *PGClient) RunDropQuery() error {
	return runTimedQuery(c.db.NewDropTable().Model((*DummyTable)(nil)).IfExists().Exec)
}

// RunTruncateQuery truncates a table.
func (c *PGClient) RunTruncateQuery() error {
	return runTimedQuery(c.db.NewTruncateTable().Model(dummyModel).Exec)
}

// RunInsertQuery inserts a new row in the table.
func (c *PGClient) RunInsertQuery(id int64) error {
	model := *dummyModel
	model.ID = id
	return runTimedQuery(c.db.NewInsert().Model(&model).Exec)
}

// RunMultiInsertQuery inserts multiple values into the table.
func (c *PGClient) RunMultiInsertQuery(values ...string) error {

	entries := make([]DummyTable, 0, len(values))
	for _, value := range values {
		entries = append(entries, DummyTable{Foo: value})
	}
	return runTimedQuery(c.db.NewInsert().Model(&entries).Exec)
}

// RunSelectQuery runs a SELECT query on the test DB.
func (c *PGClient) RunSelectQuery() error {
	return c.RunSelectQueryWithLimit(0)

}

// RunSelectQueryWithLimit runs a SELECT query on the test DB with a limit on the number of rows to return.
func (c *PGClient) RunSelectQueryWithLimit(limit int) error {
	statement := c.db.NewSelect()
	if limit > 0 {
		statement = statement.Limit(limit)
	}
	return runTimedQuery(statement.Model(dummyModel).Exec)
}

// RunUpdateQuery runs an UPDATE query on the test DB.
func (c *PGClient) RunUpdateQuery() error {
	newModel := *dummyModel
	newModel.Foo = "baz"

	return runTimedQuery(c.db.NewUpdate().Model(&newModel).WherePK().Exec)
}

// optionOrDefault returns the fallback value if the option is empty.
func optionOrDefault(option, emptyOption, fallback string) string {
	if option == emptyOption {
		return fallback
	}
	return option
}

// runTimedQuery runs a query with a timeout.
func runTimedQuery(callback func(context.Context, ...interface{}) (sql.Result, error)) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := callback(ctx)
	return err
}

var (
	mapTypesToZero = map[ebpf.MapType]struct{}{
		ebpf.PerCPUArray: {},
		ebpf.Array:       {},
		ebpf.PerCPUHash:  {},
	}
)

// CleanPostgresMaps deletes all entries from the postgres maps. Test utility to allow reusing USM instance without caring
// over previous data.
func CleanPostgresMaps(t *testing.T) {
	if Spec.Instance == nil {
		t.Log("postgres protocol not initialized")
		return
	}

	m := Spec.Instance.(*protocol).mgr
	if m == nil {
		t.Log("postgres manager not initialized")
		return
	}

	// Getting all maps loaded into the manager
	maps, err := m.GetMaps()
	if err != nil {
		t.Logf("failed to get maps: %v", err)
		return
	}
	for mapName, mapInstance := range maps {
		// We only want to clean postgres maps
		if !strings.Contains(mapName, "postgres") {
			continue
		}
		// Special case for batches, as the values is never "empty", but contain the CPU number.
		if strings.HasSuffix(mapName, "postgres_batches") {
			continue
		}
		_, shouldOnlyZero := mapTypesToZero[mapInstance.Type()]

		key := make([]byte, mapInstance.KeySize())
		value := make([]byte, mapInstance.ValueSize())
		mapEntries := mapInstance.Iterate()
		var keys [][]byte
		for mapEntries.Next(&key, &value) {
			keys = append(keys, key)
		}

		if shouldOnlyZero {
			emptyValue := make([]byte, mapInstance.ValueSize())
			for _, key := range keys {
				if err := mapInstance.Put(&key, &emptyValue); err != nil {
					t.Log("failed zeroing map entry; error: ", err)
				}
			}
		} else {
			for _, key := range keys {
				if err := mapInstance.Delete(&key); err != nil {
					t.Log("failed deleting map entry; error: ", err)
				}
			}
		}
	}
}

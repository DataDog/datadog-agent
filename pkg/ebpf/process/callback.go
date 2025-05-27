// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package process

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers"
	_ "github.com/mattn/go-sqlite3"
)

// ProcessUserContext represents user context information for a process
type ProcessUserContext struct {
	PID       uint32
	UserID    string
	UserName  string
	Context   string
	Timestamp int64
}

// callbackMap is a helper struct that holds a map of callbacks and a mutex to protect it
type callbackMap struct {
	// callbacks holds the set of callbacks
	callbacks map[*consumers.ProcessCallback]struct{}

	// mutex is the mutex that protects the callbacks map
	mutex sync.RWMutex

	// hasCallbacks is a flag that indicates if there are any callbacks subscribed, used
	// to avoid locking/unlocking the mutex if there are no callbacks
	hasCallbacks atomic.Bool

	// db is the SQLite database connection
	db *sql.DB
}

func newCallbackMap() *callbackMap {
	cm := &callbackMap{
		callbacks: make(map[*consumers.ProcessCallback]struct{}),
	}

	if err := cm.initDB(); err != nil {
		// Log error but continue - the callback map will work without DB
		fmt.Printf("Failed to initialize SQLite database: %v\n", err)
	}

	return cm
}

// initDB initializes the SQLite database connection and creates the necessary table
func (c *callbackMap) initDB() error {
	dbPath := filepath.Join(os.TempDir(), "process_user_context.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Create the process_context table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS process_context (
			pid INTEGER PRIMARY KEY,
			user_id TEXT,
			user_name TEXT,
			context TEXT,
			timestamp INTEGER
		)
	`)
	if err != nil {
		db.Close()
		return fmt.Errorf("failed to create table: %w", err)
	}

	c.db = db
	return nil
}

// StoreUserContext stores user context information for a process
func (c *callbackMap) StoreUserContext(ctx ProcessUserContext) error {
	if c.db == nil {
		return fmt.Errorf("database not initialized")
	}

	_, err := c.db.Exec(`
		INSERT OR REPLACE INTO process_context (pid, user_id, user_name, context, timestamp)
		VALUES (?, ?, ?, ?, ?)
	`, ctx.PID, ctx.UserID, ctx.UserName, ctx.Context, ctx.Timestamp)

	if err != nil {
		return fmt.Errorf("failed to store user context: %w", err)
	}

	return nil
}

// GetUserContext retrieves user context information for a process
func (c *callbackMap) GetUserContext(pid uint32) (*ProcessUserContext, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	ctx := &ProcessUserContext{}
	err := c.db.QueryRow(`
		SELECT pid, user_id, user_name, context, timestamp
		FROM process_context
		WHERE pid = ?
	`, pid).Scan(&ctx.PID, &ctx.UserID, &ctx.UserName, &ctx.Context, &ctx.Timestamp)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user context: %w", err)
	}

	return ctx, nil
}

// Close closes the database connection
func (c *callbackMap) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// add adds a callback to the callback map and returns a function that can be called to remove it
func (c *callbackMap) add(cb consumers.ProcessCallback) func() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.callbacks[&cb] = struct{}{}
	c.hasCallbacks.Store(true)

	return func() {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		delete(c.callbacks, &cb)
		c.hasCallbacks.Store(len(c.callbacks) > 0)
	}
}

func (c *callbackMap) call(pid uint32) {
	if !c.hasCallbacks.Load() {
		return
	}

	c.mutex.RLock()
	defer c.mutex.RUnlock()
	for cb := range c.callbacks {
		(*cb)(pid)
	}
}

// QueryUserContext retrieves user context information based on a WHERE clause
func (c *callbackMap) QueryUserContext(whereClause string, args ...interface{}) ([]ProcessUserContext, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Build the query with the provided WHERE clause
	query := fmt.Sprintf(`
		SELECT pid, user_id, user_name, context, timestamp
		FROM process_context
		WHERE %s
	`, whereClause)

	// Execute the query with the provided arguments
	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query user context: %w", err)
	}
	defer rows.Close()

	var results []ProcessUserContext
	for rows.Next() {
		var ctx ProcessUserContext
		err := rows.Scan(&ctx.PID, &ctx.UserID, &ctx.UserName, &ctx.Context, &ctx.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		results = append(results, ctx)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration: %w", err)
	}

	return results, nil
}

// QueryUserContextUnsafe allows direct SQL queries for process context (USE WITH CAUTION)
func (c *callbackMap) QueryUserContextUnsafe(query string) ([]ProcessUserContext, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Execute the raw query
	rows, err := c.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	var results []ProcessUserContext
	for rows.Next() {
		var ctx ProcessUserContext
		err := rows.Scan(&ctx.PID, &ctx.UserID, &ctx.UserName, &ctx.Context, &ctx.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		results = append(results, ctx)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration: %w", err)
	}

	return results, nil
}

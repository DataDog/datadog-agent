// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	go_ora "github.com/sijms/go-ora/v2"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/DataDog/datadog-agent/pkg/collector/remotequeries/session"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// remoteCancelGrace bounds how long a cancelled statement may take to stop after the
// driver asked the database to cancel it, before the run closes its sockets.
const remoteCancelGrace = 5 * time.Second

// remoteSessionSettings fix how the run's session converts values to text, for
// conversions the query itself makes (QUERY-04). They do not change how result values
// are encoded, which the integration does from the driver's values.
const remoteSessionSettings = `ALTER SESSION SET TIME_ZONE = 'UTC' NLS_NUMERIC_CHARACTERS = '.,' ` +
	`NLS_DATE_FORMAT = 'YYYY-MM-DD"T"HH24:MI:SS' NLS_TIMESTAMP_FORMAT = 'YYYY-MM-DD"T"HH24:MI:SS.FF' ` +
	`NLS_TIMESTAMP_TZ_FORMAT = 'YYYY-MM-DD"T"HH24:MI:SS.FFTZH:TZM'`

var oraCodePattern = regexp.MustCompile(`ORA-[0-9]{5}`)

// remoteDB is a run's connection pool of one connection, with the means to close its
// sockets whatever the driver is blocked on.
type remoteDB struct {
	*sql.DB
	// kill closes every socket of the run's connection. It is safe to call repeatedly
	// and concurrently.
	kill func()
}

// remoteDependencies are what a run uses outside the check. Configure sets the
// production ones; tests replace them.
type remoteDependencies struct {
	openDB     func(cfg config.ConnectionConfig, timeout time.Duration) (*remoteDB, error)
	apiKey     func() string
	httpClient func() (*http.Client, func())
}

func defaultRemoteDependencies() *remoteDependencies {
	return &remoteDependencies{
		openDB: openRemoteOracle,
		apiKey: func() string { return strings.TrimSpace(pkgconfigsetup.Datadog().GetString("api_key")) },
		httpClient: func() (*http.Client, func()) {
			return session.NewHTTPClient(pkgconfigsetup.Datadog())
		},
	}
}

// socketTracker records the run's sockets so they can be closed on cancellation.
type socketTracker struct {
	mu     sync.Mutex
	closed bool
	conns  []net.Conn
}

func (t *socketTracker) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		_ = conn.Close()
		return nil, net.ErrClosed
	}
	t.conns = append(t.conns, conn)
	return conn, nil
}

func (t *socketTracker) closeAll() {
	t.mu.Lock()
	t.closed = true
	conns := t.conns
	t.conns = nil
	t.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

// openRemoteOracle opens a go-ora pool of one connection for one run, with the check's
// connection settings, including TLS and wallet (QUERY-01). The driver's read timeout
// is the run's remaining database time, not the monitoring query_timeout.
func openRemoteOracle(cfg config.ConnectionConfig, timeout time.Duration) (*remoteDB, error) {
	seconds := max(1, int((timeout+time.Second-1)/time.Second))
	dsn := goOraURL(cfg, map[string]string{"TIMEOUT": strconv.Itoa(seconds)})
	connector, ok := go_ora.NewConnector(dsn).(*go_ora.OracleConnector)
	if !ok {
		return nil, errors.New("unsupported driver connector")
	}
	tracker := &socketTracker{}
	connector.Dialer(tracker)
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	return &remoteDB{DB: db, kill: tracker.closeAll}, nil
}

// remoteRun is one execute call.
type remoteRun struct {
	check     *Check
	req       session.Request
	deps      *remoteDependencies
	received  time.Time
	logPrefix string
	failure   *session.Failure
	pages     int
	rows      int64
}

// execute returns the run's final answer. Every run that fails after its request was
// validated aborts its upload session with the failure (ABORT-01).
func (r *remoteRun) execute(parent context.Context) (answer string) {
	d := r.req.Delivery
	ctx, cancel := context.WithDeadline(parent, r.received.Add(time.Duration(d.Limits.TimeoutMs)*time.Millisecond))
	defer cancel()
	var intake *session.Client
	finalized := false

	defer func() {
		if p := recover(); p != nil {
			// Frames only: the panic value could carry query text or data.
			log.Errorf("%s panic: %s", r.logPrefix, debug.Stack())
			r.failure = session.Fail(session.CodeInternalError, "the remote query failed unexpectedly")
		}
		if finalized {
			answer = session.Succeeded()
			return
		}
		if r.failure == nil {
			r.failure = session.Fail(session.CodeInternalError, "the remote query failed unexpectedly")
		}
		if intake != nil {
			if err := intake.Abort(r.failure); err != nil {
				log.Warnf("%s abort failed: %s", r.logPrefix, abortErrorCode(err))
			}
		} else {
			log.Warnf("%s not aborted: the Agent API key is unavailable", r.logPrefix)
		}
		answer = session.Failed(r.failure)
	}()

	apiKey := r.deps.apiKey()
	if apiKey == "" {
		r.failure = session.Fail(session.CodeCredentialsUnavailable, "the Agent API key is unavailable")
		return ""
	}
	client, closeClient := r.deps.httpClient()
	defer closeClient()
	intake = session.NewClient(client, d, apiKey, r.req.Trace)

	// Match again, then check the statement and the Agent's identity, all before
	// connecting (MATCH-05, QUERY-02).
	if f := r.check.remoteMatch(r.req.Target); f != nil {
		r.failure = f
		return ""
	}
	if !remoteQueryIsSelect(r.req.Query) {
		r.failure = session.Fail(codeUnsupportedQuery, "the query is not a single SELECT or WITH query; remote queries run queries only")
		return ""
	}
	hostname := r.check.agentHostname
	if hostname == "" {
		r.failure = session.Fail(session.CodeInternalError, "the Agent hostname is unavailable")
		return ""
	}

	uploader, err := r.stream(ctx, intake, hostname)
	if uploader != nil {
		r.pages, r.rows = uploader.Pages(), uploader.Rows()
	}
	if err != nil {
		r.failure = r.classify(ctx, err)
		return ""
	}
	finalized = true
	return ""
}

func abortErrorCode(err error) string {
	var ie *session.IntakeError
	if errors.As(err, &ie) {
		return ie.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "transport_error"
}

// errDBTimeout marks the end of the database time that the instance's
// remote_queries.timeout_ms set (QUERY-05).
var errDBTimeout = errors.New("remote query database time limit")

// classify maps an error to the run's failure. Cancellation and time limits win over
// the secondary errors they cause.
func (r *remoteRun) classify(ctx context.Context, err error) *session.Failure {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(err, errDBTimeout):
		return session.Fail(session.CodeTimeout, "the remote query exceeded its time limit")
	case ctx.Err() != nil:
		return session.Fail(session.CodeCancelled, "the remote query was cancelled")
	}
	var f *session.Failure
	if errors.As(err, &f) {
		return f
	}
	var ie *session.IntakeError
	if errors.As(err, &ie) {
		return session.Fail(session.CodeUploadFailed, "intake request failed: "+ie.Code)
	}
	return session.Fail(session.CodeInternalError, "the remote query failed unexpectedly")
}

// databaseFailure carries only the Oracle error code of a driver error: driver text
// can include hosts, SQL, or data (QUERY-06).
func databaseFailure(code, message string, err error) *session.Failure {
	if ora := oraCodePattern.FindString(err.Error()); ora != "" {
		message += " (" + ora + ")"
	}
	return session.Fail(code, message)
}

// dbDeadline is when the run's database time ends: the run time limit, or the
// instance's remote query timeout when that is sooner (QUERY-05).
func (r *remoteRun) dbDeadline(ctx context.Context) time.Time {
	deadline, _ := ctx.Deadline()
	if t := r.check.config.RemoteQueries.TimeoutMs; t != nil {
		if own := time.Now().Add(time.Duration(*t) * time.Millisecond); own.Before(deadline) {
			return own
		}
	}
	return deadline
}

// stream runs the query in a read-only transaction on the run's own connection and
// uploads its rows page by page, then finalizes the session.
func (r *remoteRun) stream(ctx context.Context, intake *session.Client, hostname string) (*session.Uploader, error) {
	dbDeadline := r.dbDeadline(ctx)
	dbCtx, cancelDB := context.WithDeadlineCause(ctx, dbDeadline, errDBTimeout)
	defer cancelDB()
	db, err := r.deps.openDB(r.check.config.ConnectionConfig, time.Until(dbDeadline))
	if err != nil {
		return nil, databaseFailure(session.CodeConnectionFailed, "the database connection could not be opened", err)
	}
	// When the run is cancelled or its database time ends, the driver asks the
	// database to cancel the statement; the sockets are closed if it has not stopped
	// within the grace time (CANCEL-01).
	var graceTimer *time.Timer
	var graceMu sync.Mutex
	stopWatch := context.AfterFunc(dbCtx, func() {
		graceMu.Lock()
		defer graceMu.Unlock()
		graceTimer = time.AfterFunc(remoteCancelGrace, db.kill)
	})
	closeDB := sync.OnceFunc(func() {
		stopWatch()
		graceMu.Lock()
		if graceTimer != nil {
			graceTimer.Stop()
		}
		graceMu.Unlock()
		_ = db.Close() // best effort: the sockets are closed next regardless
		db.kill()
	})
	defer closeDB()

	conn, tx, rows, err := r.startQuery(dbCtx, db)
	if err != nil {
		return nil, r.dbError(dbCtx, err)
	}
	defer func() { _ = conn.Close() }()
	defer func() { _ = tx.Rollback() }() // best effort: a finished transaction reports ErrTxDone
	defer rows.Close()

	columns, err := r.describe(rows)
	if err != nil {
		return nil, err
	}
	sessionColumns := make([]session.Column, len(columns))
	for i, c := range columns {
		sessionColumns[i] = c.Column
	}
	uploader, failure := session.NewUploader(intake, r.req.Delivery, sessionColumns, r.req.IncludeSchema, hostname, r.logPrefix)
	if failure != nil {
		return nil, failure
	}
	if err := uploader.Register(ctx); err != nil {
		return uploader, err
	}
	if err := r.pump(dbCtx, uploader, rows, columns); err != nil {
		return uploader, r.dbError(dbCtx, err)
	}
	// Every row is read: release the database before the last page and the finalize.
	if err := rows.Close(); err != nil {
		return uploader, r.dbError(dbCtx, databaseFailure(session.CodeQueryFailed, "the database cursor could not be closed", err))
	}
	_ = tx.Rollback() // read-only: nothing to keep
	_ = conn.Close()
	closeDB()
	return uploader, uploader.Finish(ctx)
}

// dbError reports the database time limit instead of the driver error it causes.
func (r *remoteRun) dbError(dbCtx context.Context, err error) error {
	if errors.Is(context.Cause(dbCtx), errDBTimeout) {
		return errDBTimeout
	}
	return err
}

// startQuery fixes the session's settings, starts the read-only transaction, and
// starts the query, all on the run's one connection. On error it leaves nothing open.
func (r *remoteRun) startQuery(ctx context.Context, db *remoteDB) (*sql.Conn, *sql.Tx, *sql.Rows, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, nil, nil, databaseFailure(session.CodeConnectionFailed, "the database connection could not be opened", err)
	}
	fail := func(tx *sql.Tx, f *session.Failure) (*sql.Conn, *sql.Tx, *sql.Rows, error) {
		if tx != nil {
			_ = tx.Rollback() // best effort on an already failed run
		}
		_ = conn.Close()
		return nil, nil, nil, f
	}
	if _, err := conn.ExecContext(ctx, remoteSessionSettings); err != nil {
		return fail(nil, databaseFailure(session.CodeQueryFailed, "the database refused the session settings", err))
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fail(nil, databaseFailure(session.CodeConnectionFailed, "the database connection could not be opened", err))
	}
	// go-ora does not support sql.TxOptions{ReadOnly}: ask the database. This must be
	// the transaction's first statement.
	if _, err := tx.ExecContext(ctx, "SET TRANSACTION READ ONLY"); err != nil {
		return fail(tx, databaseFailure(session.CodeQueryFailed, "the database refused a read-only transaction", err))
	}
	rows, err := tx.QueryContext(ctx, r.req.Query)
	if err != nil {
		return fail(tx, databaseFailure(session.CodeQueryFailed, "the database rejected the query", err))
	}
	return conn, tx, rows, nil
}

// describe maps the result's columns to the descriptor, rejecting types it cannot
// carry (DESC-01, DESC-02, ORA-01).
func (r *remoteRun) describe(rows *sql.Rows) ([]remoteColumn, error) {
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, databaseFailure(session.CodeQueryFailed, "the result columns could not be read", err)
	}
	columns := make([]remoteColumn, len(types))
	for i, t := range types {
		precision, scale, hasScale := t.DecimalSize()
		col, failure := remoteColumnType(t.Name(), t.DatabaseTypeName(), precision, scale, hasScale)
		if failure != nil {
			return nil, failure
		}
		columns[i] = col
	}
	return columns, nil
}

// pump reads the rows, encodes them, and hands them to the uploader.
func (r *remoteRun) pump(ctx context.Context, uploader *session.Uploader, rows *sql.Rows, columns []remoteColumn) error {
	values := make([]any, len(columns))
	dest := make([]any, len(columns))
	for i := range values {
		dest[i] = &values[i]
	}
	cells := make([]session.Cell, len(columns))
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := rows.Scan(dest...); err != nil {
			return databaseFailure(session.CodeQueryFailed, "a result row could not be read", err)
		}
		for i, col := range columns {
			cell, failure := remoteCell(col.kind, values[i])
			if failure != nil {
				return session.Fail(failure.Code, failure.Message+" in column "+quoteColumn(col.Name))
			}
			cells[i] = cell
		}
		if err := uploader.Add(ctx, cells); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return databaseFailure(session.CodeQueryFailed, "the database failed while returning rows", err)
	}
	return nil
}

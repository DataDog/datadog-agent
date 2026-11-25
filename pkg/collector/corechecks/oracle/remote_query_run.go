// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	go_ora "github.com/sijms/go-ora/v2"
)

// remoteDBConfig is everything needed to open the run's own connection. It is built
// from the check's configuration at execute time and never logged.
type remoteDBConfig struct {
	Server      string
	Port        int
	ServiceName string
	Username    string
	Password    string
	Timeout     time.Duration
}

// remoteDB is a run-scoped connection pool of one connection plus the means to tear
// its sockets down regardless of what the driver is blocked on.
type remoteDB struct {
	*sql.DB
	// kill closes every socket the run's connections opened. It is safe to call
	// repeatedly and concurrently.
	kill func()
}

// remoteQueryHooks are the production dependencies of a run. Tests replace them; the
// zero value of Check uses the defaults, so the production path has no test branch.
type remoteQueryHooks struct {
	openDB          func(cfg remoteDBConfig) (*remoteDB, error)
	apiKey          func() string
	httpClient      func() (*http.Client, func())
	validateBaseURL func(baseURL string) bool
}

func defaultRemoteQueryHooks() remoteQueryHooks {
	return remoteQueryHooks{
		openDB: openRemoteOracle,
		apiKey: func() string { return strings.TrimSpace(pkgconfigsetup.Datadog().GetString("api_key")) },
		httpClient: func() (*http.Client, func()) {
			transport := httputils.CreateHTTPTransport(pkgconfigsetup.Datadog())
			// The upload carries the API key: always verify the intake certificate, even if
			// monitoring traffic is configured to skip verification.
			if transport.TLSClientConfig != nil {
				transport.TLSClientConfig.InsecureSkipVerify = false
			}
			client := &http.Client{
				Transport: transport,
				// Each request also has its own, shorter, context timeout.
				Timeout: intakePageTimeout + intakeRequestTimeout,
				// Never follow a redirect with the API key attached.
				CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
			}
			return client, transport.CloseIdleConnections
		},
		validateBaseURL: func(baseURL string) bool {
			want, ok := intakeSiteBaseURL(pkgconfigsetup.Datadog().GetString("site"))
			return ok && baseURL == want
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

// openRemoteOracle opens a fresh go-ora pool for one run: the configured direct TCP
// server and explicit service only, no monitoring pool or state. The driver's per-read
// TIMEOUT is the run's remaining time, not the monitoring query_timeout.
func openRemoteOracle(cfg remoteDBConfig) (*remoteDB, error) {
	seconds := max(1, int((cfg.Timeout+time.Second-1)/time.Second))
	dsn := go_ora.BuildUrl(cfg.Server, cfg.Port, cfg.ServiceName, cfg.Username, cfg.Password, map[string]string{"TIMEOUT": strconv.Itoa(seconds)})
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
	check   *Check
	req     remoteRequest
	hooks   remoteQueryHooks
	failure *remoteFailure
	pages   int
	rows    int64
}

func (r *remoteRun) fail(code, message string) {
	r.failure = newRemoteFailure(code, message)
}

// execute returns the final answer. Every path that does not finalize the upload
// session aborts it, so the worker learns the outcome without waiting for expiry.
func (r *remoteRun) execute(parent context.Context) (answer string) {
	req := r.req
	d := req.Delivery
	apiKey := r.hooks.apiKey()
	var intake *intakeClient
	finalized := false

	defer func() {
		if p := recover(); p != nil {
			// Frames only: the panic value could carry query text or data.
			log.Errorf("oracle remote query panic: %s", debug.Stack())
			r.fail(remoteCodeInternalError, "the remote query failed unexpectedly")
			answer = remoteFailed(remoteCodeInternalError, "the remote query failed unexpectedly")
		}
		if finalized {
			return
		}
		if intake != nil {
			// Best effort; the run may already be past its deadline or cancelled.
			_ = intake.abort()
		}
		if r.failure == nil {
			r.fail(remoteCodeInternalError, "the remote query failed unexpectedly")
		}
		answer = remoteFailed(r.failure.code, r.failure.message)
	}()

	// Revalidate from configuration, then choose the statement: both before any
	// connection or upload request. A bad delivery cannot be aborted safely, so only
	// an upload-capable run aborts its session below.
	if apiKey == "" {
		r.fail(remoteCodeCredentials, "the Agent API key is unavailable")
		return ""
	}
	if !r.hooks.validateBaseURL(d.BaseURL) {
		r.fail(remoteCodeInvalidRequest, "the upload address is not the Agent's own intake")
		return ""
	}
	client, closeClient := r.hooks.httpClient()
	defer closeClient()
	intake = newIntakeClient(client, d.BaseURL, d.UploadID, apiKey, req.Trace)

	if !remoteServes(r.check.config, req.Target) {
		r.fail(remoteCodeTargetNotFound, "this Oracle check does not serve the target")
		return ""
	}
	statement, ok := approvedRemoteQuery(req.Query)
	if !ok {
		r.fail(remoteCodeUnsupportedQuery, "the query is not one of the fixed queries this integration runs")
		return ""
	}
	hostname := r.check.agentHostname
	if hostname == "" {
		r.fail(remoteCodeInternalError, "the Agent hostname is unavailable")
		return ""
	}

	ctx, cancel := context.WithTimeout(parent, time.Duration(d.Limits.TimeoutMs)*time.Millisecond)
	defer cancel()
	if err := r.stream(ctx, intake, statement, hostname); err != nil {
		r.failure = r.classify(ctx, err)
		return ""
	}
	finalized = true
	return encodeRemoteAnswer(remoteAnswer{Status: "SUCCEEDED"})
}

// classify maps an error to a final failure. Cancellation and the run deadline win over
// the secondary errors they cause.
func (r *remoteRun) classify(ctx context.Context, err error) *remoteFailure {
	var rf *remoteFailure
	if errors.As(err, &rf) && ctx.Err() == nil {
		return rf
	}
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return newRemoteFailure(remoteCodeTimeout, "the remote query exceeded its time limit")
	case ctx.Err() != nil:
		return newRemoteFailure(remoteCodeCancelled, "the remote query was cancelled")
	}
	var ie *intakeError
	if errors.As(err, &ie) {
		return newRemoteFailure(remoteCodeUploadFailed, "intake request failed: "+ie.code)
	}
	return newRemoteFailure(remoteCodeInternalError, "the remote query failed unexpectedly")
}

func databaseFailure(code, message string, err error) *remoteFailure {
	// Only an Oracle error code is surfaced; driver text can carry hosts, SQL or data.
	if ora := remoteOraCodeRegexp.FindString(err.Error()); ora != "" {
		message += " (" + ora + ")"
	}
	return newRemoteFailure(code, message)
}

func (r *remoteRun) dbConfig(timeout time.Duration) remoteDBConfig {
	c := r.check.config.ConnectionConfig
	return remoteDBConfig{Server: c.Server, Port: c.Port, ServiceName: strings.TrimSpace(c.ServiceName), Username: c.Username, Password: c.Password, Timeout: timeout}
}

// stream runs the statement in a read-only transaction on a dedicated connection and
// uploads its rows page by page, then finalizes the session.
func (r *remoteRun) stream(ctx context.Context, intake *intakeClient, statement, hostname string) error {
	deadline, _ := ctx.Deadline()
	db, err := r.hooks.openDB(r.dbConfig(time.Until(deadline)))
	if err != nil {
		return databaseFailure(remoteCodeConnectionFailed, "the database connection could not be opened", err)
	}
	// Whatever the driver is blocked on, cancellation closes the sockets.
	stopWatch := context.AfterFunc(ctx, db.kill)
	defer func() {
		stopWatch()
		_ = db.Close() // best effort: the sockets are closed next regardless
		db.kill()
	}()

	tx, rows, err := r.startQuery(ctx, db, statement)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // best effort: a finished transaction reports ErrTxDone
	defer rows.Close()

	columns, err := r.describe(rows, r.req.Delivery.Limits)
	if err != nil {
		return err
	}
	page, err := r.registerDescriptor(ctx, intake, columns, hostname)
	if err != nil {
		return err
	}
	if err := r.pump(ctx, intake, rows, columns, page); err != nil {
		return err
	}
	// All rows are read: release the database before finalizing.
	if err := rows.Close(); err != nil {
		return databaseFailure(remoteCodeQueryFailed, "the database cursor could not be closed", err)
	}
	_ = tx.Rollback() // read-only: nothing to keep
	_ = db.Close()
	return intake.finalize(ctx, r.pages)
}

// startQuery begins the run's transaction, makes it read-only and starts the fixed
// statement. On error it leaves nothing open.
func (r *remoteRun) startQuery(ctx context.Context, db *remoteDB, statement string) (*sql.Tx, *sql.Rows, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, databaseFailure(remoteCodeConnectionFailed, "the database connection could not be opened", err)
	}
	// go-ora does not support sql.TxOptions{ReadOnly}: ask the database explicitly. This
	// must be the transaction's first statement. The approved fixed statement is the
	// primary guard; this is defence in depth.
	if _, err := tx.ExecContext(ctx, "SET TRANSACTION READ ONLY"); err != nil {
		_ = tx.Rollback() // best effort on an already failed run
		return nil, nil, databaseFailure(remoteCodeQueryFailed, "the database refused a read-only transaction", err)
	}
	rows, err := tx.QueryContext(ctx, statement)
	if err != nil {
		_ = tx.Rollback() // best effort on an already failed run
		return nil, nil, databaseFailure(remoteCodeQueryFailed, "the database rejected the query", err)
	}
	return tx, rows, nil
}

// describe maps the result columns to the descriptor, rejecting anything unsupported.
func (r *remoteRun) describe(rows *sql.Rows, limits remoteLimits) ([]remoteColumn, error) {
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, databaseFailure(remoteCodeQueryFailed, "the result columns could not be read", err)
	}
	if len(types) == 0 || int64(len(types)) > limits.MaxColumns {
		return nil, newRemoteFailure(remoteCodeInvalidColumns, "the result has no columns or exceeds the column limit")
	}
	columns := make([]remoteColumn, len(types))
	seen := make(map[string]struct{}, len(types))
	for i, t := range types {
		col, failure := classifyRemoteColumn(t.Name(), t.DatabaseTypeName())
		if failure != nil {
			return nil, failure
		}
		if _, dup := seen[col.Name]; dup {
			return nil, newRemoteFailure(remoteCodeInvalidColumns, "the result has duplicate column names")
		}
		seen[col.Name] = struct{}{}
		columns[i] = col
	}
	if r.req.IncludeSchema && int64(schemaJSONBytes(columns)) > limits.MaxSchemaBytes {
		return nil, newRemoteFailure(remoteCodeDescriptorTooLarge, "the result schema exceeds its size limit")
	}
	return columns, nil
}

// registerDescriptor sends the descriptor and returns the page encoder for the columns.
func (r *remoteRun) registerDescriptor(ctx context.Context, intake *intakeClient, columns []remoteColumn, hostname string) (*pageEncoder, error) {
	d := r.req.Delivery
	body, err := json.Marshal(newRemoteDescriptor(columns, r.req.IncludeSchema, hostname))
	if err != nil {
		return nil, newRemoteFailure(remoteCodeInternalError, "the descriptor could not be encoded")
	}
	page, failure := newPageEncoder(columns, r.req.IncludeSchema, d.CrawlID, d.TaskID, hostname)
	if failure != nil {
		return nil, failure
	}
	if page.emptyBound() > d.Limits.MaxFileBytes {
		return nil, newRemoteFailure(remoteCodeResultTooLarge, "the page envelope exceeds the result file limit")
	}
	if err := intake.registerDescriptor(ctx, body); err != nil {
		return nil, err
	}
	return page, nil
}

// pump reads rows, encodes them and uploads each full page, then the last one. An
// empty result is one empty page, so every finalized session has a file.
func (r *remoteRun) pump(ctx context.Context, intake *intakeClient, rows *sql.Rows, columns []remoteColumn, page *pageEncoder) error {
	limits := r.req.Delivery.Limits
	target := min(limits.MaxFileBytes, remoteMaxPageTargetBytes)
	var offset, resultBytes int64
	flush := func() error {
		if int64(r.pages) >= limits.MaxPages || resultBytes+page.pageBound() > limits.MaxResultBytes || int64(page.body.Len()) > remoteMaxSourcePageBytes {
			return newRemoteFailure(remoteCodeResultTooLarge, "the result exceeds its page or total size limit")
		}
		if err := intake.putPage(ctx, r.pages, page.bytes(), page.rows, offset); err != nil {
			if isIntakeCode(err, "result_file_too_large") {
				return newRemoteFailure(remoteCodeRowTooLarge, "a page exceeded intake's result file limit")
			}
			return err
		}
		r.pages++
		r.rows += page.rows
		offset += page.rows
		resultBytes += page.pageBound()
		page.reset()
		return nil
	}

	values := make([]any, len(columns))
	dest := make([]any, len(columns))
	for i := range values {
		dest[i] = &values[i]
	}
	tokens := make([]string, len(columns))
	bounds := make([]int, len(columns))
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := rows.Scan(dest...); err != nil {
			return databaseFailure(remoteCodeQueryFailed, "a result row could not be read", err)
		}
		for i, col := range columns {
			token, bound, failure := encodeRemoteCell(col.kind, values[i])
			if failure != nil {
				return failure
			}
			tokens[i], bounds[i] = token, bound
		}
		rowBound := page.rowBound(bounds)
		if rowBound > limits.MaxRowBytes || int64(page.overhead)+rowBound > limits.MaxFileBytes {
			return newRemoteFailure(remoteCodeRowTooLarge, "a result row exceeds the row or file size limit")
		}
		if !page.empty() && page.fileBound(rowBound) > target {
			if err := flush(); err != nil {
				return err
			}
		}
		if failure := page.add(tokens, rowBound); failure != nil {
			return failure
		}
	}
	if err := rows.Err(); err != nil {
		return databaseFailure(remoteCodeQueryFailed, "the database failed while returning rows", err)
	}
	if !page.empty() || r.pages == 0 {
		return flush()
	}
	return nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/DataDog/datadog-agent/pkg/collector/remotequeries/session"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// This file is the Remote Queries boundary of the Oracle check (check.RemoteQuerier):
// matching a target from the check's configuration, admitting runs, and stopping them
// when the check is unscheduled. The specification is docs/spec.md.

// Failure codes of the Oracle integration, besides the shared ones (session.Code*).
const (
	codeUnsupportedQuery = "unsupported_query"
	codeUnsupportedValue = "unsupported_value"
)

// remoteQueryState tracks the runs admitted on one check so unscheduling can stop them.
type remoteQueryState struct {
	mu      sync.Mutex
	closed  bool
	next    uint64
	cancels map[uint64]context.CancelFunc
	wg      sync.WaitGroup
	// identifier is the check's database identifier once its first run has
	// initialized it; database instance targets match it (MATCH-02).
	identifier atomic.Pointer[string]
}

func (s *remoteQueryState) admit(ctx context.Context) (context.Context, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, nil, check.ErrRemoteQueryCancelled
	}
	runCtx, cancel := context.WithCancel(ctx)
	if s.cancels == nil {
		s.cancels = map[uint64]context.CancelFunc{}
	}
	id := s.next
	s.next++
	s.cancels[id] = cancel
	s.wg.Add(1)
	return runCtx, func() {
		s.mu.Lock()
		delete(s.cancels, id)
		s.mu.Unlock()
		cancel()
		s.wg.Done()
	}, nil
}

// close refuses new calls, cancels runs in progress, and waits for them to end.
func (s *remoteQueryState) close() {
	s.mu.Lock()
	s.closed = true
	for _, cancel := range s.cancels {
		cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

// publishRemoteIdentifier makes the check's database identifier available to remote
// query matching. The scheduled run calls it once the identifier is known.
func (c *Check) publishRemoteIdentifier(identifier string) {
	c.remote.identifier.Store(&identifier)
}

// SupportsRemoteQueries reports that this check implements the remote query boundary.
func (c *Check) SupportsRemoteQueries() bool {
	return c.config != nil && c.remoteDeps != nil
}

// Cancel stops remote runs before the check is torn down: it refuses new calls,
// cancels the runs in progress, and waits for them to end (CANCEL-01 to CANCEL-03).
// Each run's remaining work is bounded: the database cancel request and one abort,
// each with its own time limit.
func (c *Check) Cancel() {
	c.remote.close()
	c.CheckBase.Cancel()
}

// RemoteQuery implements check.RemoteQuerier. Only the outcome crosses this boundary.
func (c *Check) RemoteQuery(ctx context.Context, request string) (string, error) {
	received := time.Now()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	runCtx, release, err := c.remote.admit(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	req, err := session.ParseRequest(request)
	if err != nil {
		return session.Failed(session.Fail(session.CodeInvalidRequest, "the remote query request is invalid")), nil
	}
	if req.Operation == session.OperationResolve {
		if f := c.remoteMatch(req.Target); f != nil {
			return session.Failed(f), nil
		}
		return session.Matched(), nil
	}
	return c.executeRemote(runCtx, req, received), nil
}

func normalizeRemoteHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

// remoteMatch reports whether this check serves the target, from its configuration and
// the identifier its scheduled run published only: no connection, no SQL (MATCH-03).
// It returns nil on a match. A check whose monitoring connection does not use the
// go-ora driver serves no target, so it never makes a resolution ambiguous or fail.
func (c *Check) remoteMatch(t session.Target) *session.Failure {
	notFound := session.Fail(session.CodeTargetNotFound, "no Oracle instance on this check matches the target")
	cfg := c.config
	if cfg == nil || !usesGoOra(cfg.ConnectionConfig) {
		return notFound
	}
	if t.DatabaseInstance != "" {
		identifier := c.remote.identifier.Load()
		if identifier == nil {
			return session.Fail(session.CodeTargetUnavailable, "the check has not determined its database identifier yet")
		}
		if *identifier != t.DatabaseInstance {
			return notFound
		}
		return nil
	}
	if remoteServesAddress(cfg, t) {
		return nil
	}
	return notFound
}

// remoteServesAddress matches a host, port, and service name target (MATCH-01).
func remoteServesAddress(cfg *config.CheckConfig, t session.Target) bool {
	c := cfg.ConnectionConfig
	host := normalizeRemoteHost(c.Server)
	service := strings.TrimSpace(c.ServiceName)
	if host == "" || service == "" || c.Port < 1 || c.Port > 65535 {
		return false
	}
	return host == normalizeRemoteHost(t.Host) && c.Port == t.Port && strings.EqualFold(service, strings.TrimSpace(t.DBName))
}

// executeRemote runs one query request and always returns its final answer.
func (c *Check) executeRemote(ctx context.Context, req session.Request, received time.Time) string {
	run := &remoteRun{check: c, req: req, deps: c.remoteDeps, received: received, logPrefix: "oracle remote query check=" + string(c.ID()) + " " + session.LogFields(req.Delivery)}
	answer := run.execute(ctx)
	status, code := "succeeded", ""
	if run.failure != nil {
		status, code = "failed", run.failure.Code
	}
	// Identifiers only: never SQL, credentials, connection strings, driver text, or rows.
	log.Infof("%s outcome=%s code=%s pages=%d rows=%d duration=%s", run.logPrefix, status, code, run.pages, run.rows, time.Since(received))
	return answer
}

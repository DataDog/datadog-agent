// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dcv collects Amazon DCV session and connection inventory.
package dcv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	vdimodel "github.com/DataDog/datadog-agent/pkg/vdi/model"
)

const (
	// DefaultExecutable is the protected default installation path for the DCV CLI.
	DefaultExecutable = `C:\Program Files\NICE\DCV\Server\bin\dcv.exe`
	commandTimeout    = 5 * time.Second
	cacheTTL          = 10 * time.Second
	maxOutputBytes    = 1 << 20
	maxSessions       = 128
)

var sessionLinePattern = regexp.MustCompile(`^Session:\s+(.+?)\s+\(owner:(.*?)\s+type:(?:virtual|console)(?:\s+.*)?\)$`)

// Runner executes one fixed DCV command and returns bounded output.
type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// Collector collects and briefly caches DCV inventory.
type Collector struct {
	runner Runner
	now    func() time.Time

	mu       sync.Mutex
	cachedAt time.Time
	cached   vdimodel.ProviderInventory
}

// NewCollector returns a DCV inventory collector.
func NewCollector(runner Runner) *Collector {
	return &Collector{runner: runner, now: time.Now}
}

// Provider returns the inventory provider implemented by this collector.
func (*Collector) Provider() string { return vdimodel.ProviderAWSWorkSpaces }

// Collect returns fresh or briefly cached DCV inventory. Only one collection
// can execute at a time.
func (c *Collector) Collect(ctx context.Context) vdimodel.ProviderInventory {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	if !c.cachedAt.IsZero() && now.Sub(c.cachedAt) < cacheTTL {
		return c.cached
	}

	result := c.collect(ctx)
	c.cachedAt = now
	c.cached = result
	return result
}

func (c *Collector) collect(ctx context.Context) vdimodel.ProviderInventory {
	result := vdimodel.ProviderInventory{
		SourceStatus: vdimodel.SourceStatus{
			Status: vdimodel.StatusOK,
		},
	}

	commandCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	output, err := c.runner.Run(commandCtx, "list-sessions")
	cancel()
	if err != nil {
		result.Status = vdimodel.StatusError
		result.Error = boundedError("list DCV sessions", err)
		return result
	}
	if len(output) > maxOutputBytes {
		result.Status = vdimodel.StatusError
		result.Error = "list DCV sessions: output exceeded 1 MiB"
		return result
	}

	sessions, err := parseSessions(output)
	if err != nil {
		result.Status = vdimodel.StatusError
		result.Error = boundedError("parse DCV sessions", err)
		return result
	}
	if len(sessions) > maxSessions {
		result.Status = vdimodel.StatusError
		result.Error = fmt.Sprintf("DCV session count %d exceeds limit %d", len(sessions), maxSessions)
		return result
	}

	var failures []string
	for _, session := range sessions {
		result.Sessions = append(result.Sessions, vdimodel.Session{
			ID:       session.id,
			Protocol: vdimodel.ProtocolDCV,
			Owner:    session.owner,
		})
		resultSession := &result.Sessions[len(result.Sessions)-1]

		commandCtx, cancel = context.WithTimeout(ctx, commandTimeout)
		connectionsOutput, runErr := c.runner.Run(commandCtx, "list-connections", session.id, "--json")
		cancel()
		if runErr != nil {
			failures = append(failures, fmt.Sprintf("session %q: %v", session.id, runErr))
			continue
		}
		if len(connectionsOutput) > maxOutputBytes {
			failures = append(failures, fmt.Sprintf("session %q: output exceeded 1 MiB", session.id))
			continue
		}
		connections, parseErr := parseConnections(connectionsOutput)
		if parseErr != nil {
			failures = append(failures, fmt.Sprintf("session %q: %v", session.id, parseErr))
			continue
		}
		resultSession.Connections = connections
	}

	if len(failures) > 0 {
		result.Status = vdimodel.StatusPartial
		result.Error = truncate(strings.Join(failures, "; "), 1024)
	}
	return result
}

type listedSession struct {
	id    string
	owner string
}

func parseSessions(output []byte) ([]listedSession, error) {
	var sessions []listedSession
	for _, rawLine := range strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		match := sessionLinePattern.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("unexpected line %q", truncate(line, 256))
		}
		sessionID, err := parseListedSessionID(match[1])
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, listedSession{id: sessionID, owner: match[2]})
	}
	return sessions, nil
}

// DCV 2026 encloses session IDs in single quotes in list-sessions output,
// while older versions emit the ID without quotes. Quotes are presentation
// syntax and must not be passed literally to list-connections.
func parseListedSessionID(value string) (string, error) {
	if strings.HasPrefix(value, "'") || strings.HasSuffix(value, "'") {
		if len(value) < 2 || value[0] != '\'' || value[len(value)-1] != '\'' {
			return "", fmt.Errorf("invalid quoted session id %q", value)
		}
		value = value[1 : len(value)-1]
	}
	if err := validateSessionID(value); err != nil {
		return "", err
	}
	return value, nil
}

type connectionJSON struct {
	ID                  json.Number `json:"id"`
	Username            string      `json:"username"`
	UserAgent           string      `json:"user-agent"`
	ClientMode          string      `json:"client-mode"`
	ConnectionTime      string      `json:"connection-time"`
	LastInteractionTime string      `json:"last-interaction-time"`
	FirstFrameTime      string      `json:"first-frame-time"`
	Transport           string      `json:"transport"`
}

func parseConnections(output []byte) ([]vdimodel.Connection, error) {
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.UseNumber()
	var raw []connectionJSON
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode connections: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return nil, errors.New("decode connections: trailing JSON value")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode connections: trailing data: %w", err)
	}

	connections := make([]vdimodel.Connection, 0, len(raw))
	for _, item := range raw {
		id, err := parseConnectionID(item.ID)
		if err != nil {
			return nil, err
		}
		connectedAt, err := parseOptionalTime("connection-time", item.ConnectionTime)
		if err != nil {
			return nil, err
		}
		lastInteractionAt, err := parseOptionalTime("last-interaction-time", item.LastInteractionTime)
		if err != nil {
			return nil, err
		}
		firstFrameAt, err := parseOptionalTime("first-frame-time", item.FirstFrameTime)
		if err != nil {
			return nil, err
		}
		connections = append(connections, vdimodel.Connection{
			ID:                id,
			AuthenticatedUser: item.Username,
			Transport:         strings.ToLower(item.Transport),
			ClientMode:        strings.ToLower(item.ClientMode),
			UserAgent:         item.UserAgent,
			ConnectedAt:       connectedAt,
			LastInteractionAt: lastInteractionAt,
			FirstFrameAt:      firstFrameAt,
		})
	}
	return connections, nil
}

func parseConnectionID(id json.Number) (string, error) {
	if id == "" {
		return "", errors.New("connection has no id")
	}
	parsed, err := strconv.ParseUint(string(id), 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid connection id %q", id)
	}
	return strconv.FormatUint(parsed, 10), nil
}

func parseOptionalTime(field, value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, fmt.Errorf("invalid %s %q: %w", field, value, err)
	}
	return &parsed, nil
}

func validateSessionID(id string) error {
	if id == "" || len(id) > 256 || !utf8.ValidString(id) {
		return fmt.Errorf("invalid DCV session id %q", truncate(id, 64))
	}
	if strings.HasPrefix(id, "-") {
		return fmt.Errorf("DCV session id %q begins with an option prefix", truncate(id, 64))
	}
	for _, r := range id {
		if r == 0 || r < 0x20 || r == 0x7f {
			return errors.New("DCV session id contains a control character")
		}
	}
	return nil
}

func validateCommandArgs(args []string) error {
	if len(args) == 1 && args[0] == "list-sessions" {
		return nil
	}
	if len(args) == 3 && args[0] == "list-connections" && args[2] == "--json" {
		return validateSessionID(args[1])
	}
	return errors.New("unsupported DCV command")
}

func boundedError(operation string, err error) string {
	return truncate(operation+": "+err.Error(), 1024)
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// secretBackendVersion is the wire-protocol version we send to
// secret_backend_command. The agent's own resolver uses "1.1" — see
// comp/core/secrets/def/type.go.
const secretBackendVersion = "1.1"

// defaultSecretBackendTimeout matches the agent's default and acts as a hard
// upper bound when our shell-out can't read the configured value.
const defaultSecretBackendTimeout = 30 * time.Second

// encRegexp matches a value that is exactly an ENC[handle] placeholder. The
// agent's own scanner allows any character inside the brackets except ']'.
var encRegexp = regexp.MustCompile(`^ENC\[([^\]]+)\]$`)

// resolveENC walks the resolved credential fields and, for each one that
// looks like an ENC[handle] placeholder, asks secret_backend_command to
// decrypt it. The placeholder field's source is rewritten to either
// SourceSecretBackend (success) or SourceEncrypted (failure).
//
// When SourceEncrypted survives we leave the field marked unresolved so the
// next tier of the pipeline can produce a value, falling back to env/default
// where appropriate.
func resolveENC(ctx context.Context, cfg *LiteConfig) {
	cmd := cfg.SecretBackendCommand.Value
	for _, f := range []*ConfigField{&cfg.APIKey, &cfg.Site, &cfg.DDURL} {
		if f.Value == "" {
			continue
		}
		m := encRegexp.FindStringSubmatch(f.Value)
		if m == nil {
			continue
		}

		_ = m // matched; the pipeline only needs the value, not the handle name
		if cmd == "" {
			// No backend configured. Mark the field as encrypted; the
			// pipeline's downstream tiers won't override it because we keep
			// Source=SourceEncrypted which doesn't satisfy resolved().
			f.Source = SourceEncrypted
			continue
		}

		handle := f.Value // include the brackets — the backend echoes them back
		got, err := execSecretBackend(ctx, cmd, []string{handle}, defaultSecretBackendTimeout)
		if err != nil || got[handle] == "" {
			f.Source = SourceEncrypted
			continue
		}
		f.Value = got[handle]
		f.Source = SourceSecretBackend
	}
}

// secretBackendRequest is the JSON object the backend reads from stdin.
type secretBackendRequest struct {
	Version              string   `json:"version"`
	Secrets              []string `json:"secrets"`
	SecretBackendTimeout int      `json:"secret_backend_timeout,omitempty"`
}

// secretBackendResponseEntry mirrors comp/core/secrets/def/type.go SecretVal.
type secretBackendResponseEntry struct {
	Value string `json:"value"`
	Error string `json:"error"`
}

// execSecretBackend invokes the configured secret_backend_command with the
// list of handles, parses the JSON response, and returns the resolved values.
// It is intentionally minimal (no audit log, no telemetry, no refresh): lite
// mode is a one-shot path running during agent rescue.
func execSecretBackend(ctx context.Context, command string, handles []string, timeout time.Duration) (map[string]string, error) {
	if command == "" {
		return nil, fmt.Errorf("secret_backend_command not set")
	}

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// The agent's own client allows comma-or-space-separated arg lists in
	// secret_backend_arguments. Lite mode treats the command verbatim:
	// callers can quote it themselves if they want positional args.
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("secret_backend_command is empty after splitting")
	}

	req := secretBackendRequest{
		Version:              secretBackendVersion,
		Secrets:              handles,
		SecretBackendTimeout: int(timeout / time.Second),
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	exe := exec.CommandContext(cctx, parts[0], parts[1:]...) //nolint:gosec
	exe.Stdin = bytes.NewReader(body)
	var stdout, stderr bytes.Buffer
	exe.Stdout = &stdout
	exe.Stderr = &stderr
	if err := exe.Run(); err != nil {
		return nil, fmt.Errorf("run %s: %w (stderr=%s)", parts[0], err, stderr.String())
	}

	var resp map[string]secretBackendResponseEntry
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	out := make(map[string]string, len(resp))
	for h, entry := range resp {
		if entry.Error != "" || entry.Value == "" {
			continue
		}
		out[h] = entry.Value
	}
	return out, nil
}

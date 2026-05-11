// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// secretBackendVersion matches the agent's own resolver wire-protocol version
// (see comp/core/secrets/def/type.go).
const secretBackendVersion = "1.1"

// defaultSecretBackendTimeout matches the agent's default and bounds the
// shell-out when we cannot read the configured value.
const defaultSecretBackendTimeout = 30 * time.Second

// encRegexp matches a value that is exactly an ENC[handle] placeholder. The
// agent's scanner allows any character inside the brackets except ']'.
var encRegexp = regexp.MustCompile(`^ENC\[([^\]]+)\]$`)

// resolveENC walks the credential fields and decrypts ENC[handle] placeholders
// via secret_backend_command. The field's source is rewritten to
// SourceSecretBackend (success) or SourceEncrypted (failure); SourceEncrypted
// is treated as still-unresolved so the next pipeline tier can produce a value.
func resolveENC(ctx context.Context, cfg *LiteConfig) {
	cmd := cfg.SecretBackendCommand.Value
	for _, f := range []*ConfigField{&cfg.APIKey, &cfg.Site, &cfg.DDURL} {
		if f.Value == "" {
			continue
		}
		if !encRegexp.MatchString(f.Value) {
			continue
		}
		if cmd == "" {
			f.Source = SourceEncrypted
			continue
		}

		// Include the brackets — the backend echoes them back as the key.
		handle := f.Value
		got, err := execSecretBackend(ctx, cmd, []string{handle}, defaultSecretBackendTimeout)
		if err != nil || got[handle] == "" {
			f.Source = SourceEncrypted
			continue
		}
		f.Value = got[handle]
		f.Source = SourceSecretBackend
	}
}

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
// list of handles and returns the resolved values. Intentionally minimal: no
// audit log, no telemetry, no refresh — lite mode is one-shot.
func execSecretBackend(ctx context.Context, command string, handles []string, timeout time.Duration) (map[string]string, error) {
	if command == "" {
		return nil, errors.New("secret_backend_command not set")
	}

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, errors.New("secret_backend_command is empty after splitting")
	}

	body, err := json.Marshal(secretBackendRequest{
		Version:              secretBackendVersion,
		Secrets:              handles,
		SecretBackendTimeout: int(timeout / time.Second),
	})
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

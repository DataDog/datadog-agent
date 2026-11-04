// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package privilegedhelper defines the authenticated wire protocol shared by
// rshell's privileged helper and the Datadog Private Action Runner.
package privilegedhelper

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	ProtocolVersion = 1
	MaxMessageBytes = 1 << 20
)

type KeyType string

const (
	KeyTypeX509RSA KeyType = "X509_RSA"
	KeyTypeED25519 KeyType = "ED25519"
	// KeyTypeTUFDirector identifies a request-scoped Director proof transported
	// in CredentialKey.PEM for protocol-v1 compatibility. It is never decoded as
	// a bare verification key.
	KeyTypeTUFDirector KeyType = "TUF_DIRECTOR"
)

type Signature struct {
	KeyType   KeyType `json:"keyType"`
	KeyID     string  `json:"keyId"`
	Signature []byte  `json:"signature"`
}

// SignedEnvelope contains the original backend-signed protobuf bytes. Trust
// roots are deliberately absent from the signed object. Helpers configured
// with a local Director root authenticate the request-scoped task key using
// its Director proof. Helpers without one trust the accompanying bare
// verification key; without a local policy, they use the signed backend policy
// unchanged.
type SignedEnvelope struct {
	Data       []byte      `json:"data"`
	HashType   string      `json:"hashType"`
	Signatures []Signature `json:"signatures"`
}

type ExecuteRequest struct {
	Version          int             `json:"version"`
	Envelope         SignedEnvelope  `json:"envelope"`
	VerificationKeys []CredentialKey `json:"verificationKeys,omitempty"`
	AgentPolicy      *AgentPolicy    `json:"agentPolicy,omitempty"`
}

// AgentPolicy is an unsigned, Agent-supplied authorization narrowing sent
// alongside the signed task envelope. It represents the Private Action
// Runner's local datadog.yaml restricted_shell.allowed_* operator settings,
// the same operator settings that already narrow the non-privileged rshell
// path. Because it is unsigned and supplied by a process the helper does not
// fully trust, it can only narrow (intersect with) the signed backend policy
// and any local root-owned policy.json — it can never grant a permission
// beyond what those already allow.
//
// A nil ExecuteRequest.AgentPolicy means the Agent imposes no narrowing at
// this layer at all: the effective policy is exactly signed task ∩ optional
// policy.json, identical to the protocol's behavior before this field
// existed.
//
// A non-nil AgentPolicy applies independently per field, mirroring the exact
// nil-vs-empty convention Credential (policy.json) and the Agent's own
// datadog.yaml operator settings already use elsewhere in this system: for
// each of AllowedCommands, AllowedPaths, AllowedSystemServices, and
// ElevatableCommands, a nil value means that axis is left unrestricted by
// this layer (defer entirely to signed ∩ policy.json for that axis), while a
// non-nil-but-empty value is an explicit kill switch that denies every grant
// on that axis regardless of what the signed task or policy.json allow. This
// lets an operator narrow only the axes they have configured — e.g. setting
// only AllowedCommands — without accidentally denying every system service or
// path just because those fields were left unset.
//
// These fields intentionally omit `omitempty`: Go's encoding/json treats nil
// and empty slices/maps identically when omitempty is set, which would erase
// the nil-vs-empty distinction this type depends on once it crosses the wire.
type AgentPolicy struct {
	AllowedCommands       []string            `json:"allowedCommands"`
	AllowedPaths          []string            `json:"allowedPaths"`
	AllowedSystemServices map[string][]string `json:"allowedSystemServices"`
	ElevatableCommands    []string            `json:"elevatableCommands"`
}

// NewExecuteRequest wraps an original backend-signed task for transport to the
// privileged helper. Security-relevant task fields, including the command and
// its requested effective permissions, deliberately remain inside envelope.Data
// so that the helper can authenticate them before use.
func NewExecuteRequest(envelope SignedEnvelope) ExecuteRequest {
	return ExecuteRequest{Version: ProtocolVersion, Envelope: envelope}
}

type ExecuteResponse struct {
	Version         int      `json:"version"`
	ExitCode        int      `json:"exitCode"`
	Stdout          string   `json:"stdout,omitempty"`
	Stderr          string   `json:"stderr,omitempty"`
	SandboxWarnings []string `json:"sandboxWarnings,omitempty"`
	Error           string   `json:"error,omitempty"`
}

func writeMessage(w io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	if len(payload) > MaxMessageBytes {
		return fmt.Errorf("message exceeds %d bytes", MaxMessageBytes)
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(payload)))
	if _, err := w.Write(size[:]); err != nil {
		return fmt.Errorf("write message size: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}

func readMessage(r io.Reader, value any) error {
	var size [4]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return fmt.Errorf("read message size: %w", err)
	}
	n := binary.BigEndian.Uint32(size[:])
	if n == 0 || n > MaxMessageBytes {
		return fmt.Errorf("invalid message size %d", n)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return fmt.Errorf("read message: %w", err)
	}
	dec := json.NewDecoder(io.LimitReader(&byteReader{data: payload}, int64(n)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		return fmt.Errorf("decode message: %w", err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return err
	}
	return nil
}

func ensureJSONEOF(dec *json.Decoder) error {
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("message contains trailing JSON data")
	}
	return nil
}

type byteReader struct{ data []byte }

func (r *byteReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

type Client struct {
	SocketPath string
	Timeout    time.Duration
}

// ExecuteSignedTask dispatches an original backend-signed task to the helper.
// It is the preferred client entry point because callers cannot accidentally
// select a protocol version or place authorization data outside the signature.
func (c Client) ExecuteSignedTask(ctx context.Context, envelope SignedEnvelope) (*ExecuteResponse, error) {
	return c.Execute(ctx, NewExecuteRequest(envelope))
}

func (c Client) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	if c.SocketPath == "" {
		return nil, errors.New("socket path is required")
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to privileged helper: %w", err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}
	if err := writeMessage(conn, req); err != nil {
		return nil, err
	}
	var response ExecuteResponse
	if err := readMessage(conn, &response); err != nil {
		return nil, err
	}
	if response.Version != ProtocolVersion {
		return nil, fmt.Errorf("unsupported response version %d", response.Version)
	}
	if response.Error != "" {
		return nil, errors.New(response.Error)
	}
	return &response, nil
}

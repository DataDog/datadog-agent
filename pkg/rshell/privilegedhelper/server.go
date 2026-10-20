// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privilegedhelper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

type Executor interface {
	Execute(context.Context, *VerifiedCommand) (*ExecuteResponse, error)
}

type Server struct {
	Credential  *Credential
	Executor    Executor
	Now         func() time.Time
	IdleTimeout time.Duration
	LogWriter   io.Writer
}

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	if s.Executor == nil {
		return errors.New("helper server requires an executor")
	}
	for {
		if s.IdleTimeout > 0 {
			if deadlineListener, ok := listener.(interface{ SetDeadline(time.Time) error }); ok {
				_ = deadlineListener.SetDeadline(time.Now().Add(s.IdleTimeout))
			}
		}
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return nil
			}
			return fmt.Errorf("accept helper connection: %w", err)
		}
		// Deliberately process one connection synchronously. The privileged
		// helper is a single-task executor, matching the RFC's isolation model.
		s.handle(ctx, conn)
	}
}

func (s *Server) handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	response := ExecuteResponse{Version: ProtocolVersion}
	var request ExecuteRequest
	if err := readMessage(conn, &request); err != nil {
		response.Error = err.Error()
		_ = writeMessage(conn, response)
		return
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	requestCredential := s.Credential
	var err error
	if requestCredential == nil {
		requestCredential, err = NewRequestCredential(request.VerificationKeys)
	} else if len(request.VerificationKeys) > 0 {
		requestCredential, err = requestCredential.withSocketVerificationKeys(request.VerificationKeys)
	}
	if err != nil {
		response.Error = err.Error()
		_ = writeMessage(conn, response)
		return
	}
	command, err := requestCredential.Verify(request, now)
	if err != nil {
		s.logDiagnostic("verification_failed", map[string]any{
			"error":                        err.Error(),
			"requestVersion":               request.Version,
			"signatureKeys":                signatureKeyMetadata(request.Envelope.Signatures),
			"directorProofs":               credentialKeyMetadata(request.VerificationKeys),
			"requestedAgentPolicy":         agentAuthorizationPolicy(request.AgentPolicy),
			"configuredOrgId":              requestCredential.OrgID,
			"configuredRunnerId":           requestCredential.RunnerID,
			"configuredAllowedCommands":    requestCredential.AllowedCommands,
			"configuredAllowedPaths":       requestCredential.AllowedPaths,
			"configuredElevatableCommands": requestCredential.ElevatableCommands,
			"configuredTrustedKeyCount":    len(requestCredential.decodedKeys),
			"requestTrustedKeyCount":       len(requestCredential.decodedKeys),
		})
		response.Error = err.Error()
		_ = writeMessage(conn, response)
		return
	}
	s.logDiagnostic("authorization_context", command.authorization)
	result, err := s.Executor.Execute(ctx, command)
	if err != nil {
		s.logDiagnostic("execution_failed", map[string]any{
			"taskId": command.TaskID,
			"error":  err.Error(),
		})
		response.Error = err.Error()
	} else if result != nil {
		s.logDiagnostic("execution_completed", map[string]any{
			"taskId":   command.TaskID,
			"exitCode": result.ExitCode,
		})
		response = *result
		response.Version = ProtocolVersion
	}
	_ = writeMessage(conn, response)
}

type diagnosticKey struct {
	ID   string  `json:"id"`
	Type KeyType `json:"type"`
}

func signatureKeyMetadata(signatures []Signature) []diagnosticKey {
	result := make([]diagnosticKey, 0, len(signatures))
	for _, signature := range signatures {
		result = append(result, diagnosticKey{ID: signature.KeyID, Type: signature.KeyType})
	}
	return result
}

func credentialKeyMetadata(keys []CredentialKey) []diagnosticKey {
	result := make([]diagnosticKey, 0, len(keys))
	for _, key := range keys {
		result = append(result, diagnosticKey{ID: key.ID, Type: key.Type})
	}
	return result
}

func (s *Server) logDiagnostic(event string, context any) {
	if s.LogWriter == nil {
		return
	}
	entry, err := json.Marshal(struct {
		Event   string `json:"event"`
		Context any    `json:"context"`
	}{
		Event:   event,
		Context: context,
	})
	if err != nil {
		fmt.Fprintf(s.LogWriter, "rshell privileged helper diagnostic marshal failed: %v\n", err)
		return
	}
	fmt.Fprintf(s.LogWriter, "rshell privileged helper: %s\n", entry)
}

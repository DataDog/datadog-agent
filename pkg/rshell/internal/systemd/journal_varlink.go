// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	journalRotateMethod   = "io.systemd.Journal.Rotate"
	maxVarlinkMessageSize = 64 * 1024
)

type varlinkRequest struct {
	Method string `json:"method"`
}

type varlinkReply struct {
	Parameters json.RawMessage `json:"parameters"`
	Error      string          `json:"error"`
	Continues  bool            `json:"continues"`
}

func (c *Client) rotateJournalControl(ctx context.Context, path string) error {
	conn, err := c.dialJournalControl(ctx, path)
	if err != nil {
		return err
	}
	defer conn.Close()
	return callJournalRotateVarlink(ctx, conn)
}

func callJournalRotateVarlink(ctx context.Context, conn net.Conn) error {
	stopCancel := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
	})
	defer stopCancel()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return fmt.Errorf("set journal control deadline: %w", err)
		}
	}

	request, err := json.Marshal(varlinkRequest{Method: journalRotateMethod})
	if err != nil {
		return fmt.Errorf("encode journal rotation request: %w", err)
	}
	request = append(request, 0)
	if err := writeAll(conn, request); err != nil {
		return contextIOError(ctx, "write journal rotation request", err)
	}

	message, err := readVarlinkMessage(conn)
	if err != nil {
		return contextIOError(ctx, "read journal rotation response", err)
	}
	trimmed := bytes.TrimSpace(message)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return fmt.Errorf("journal control returned a malformed response")
	}

	var reply varlinkReply
	if err := json.Unmarshal(trimmed, &reply); err != nil {
		return fmt.Errorf("decode journal rotation response: %w", err)
	}
	if reply.Continues {
		return fmt.Errorf("journal control returned an unexpected streaming response")
	}
	if reply.Error != "" {
		if !validVarlinkIdentifier(reply.Error) {
			return fmt.Errorf("journald rejected journal rotation with an invalid protocol error")
		}
		return fmt.Errorf("journald rejected journal rotation: %s", reply.Error)
	}
	if len(reply.Parameters) > 0 {
		var parameters map[string]json.RawMessage
		if err := json.Unmarshal(reply.Parameters, &parameters); err != nil || parameters == nil {
			return fmt.Errorf("journal control returned malformed response parameters")
		}
		if len(parameters) != 0 {
			return fmt.Errorf("journal control returned unexpected response parameters")
		}
	}
	return nil
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(data) {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func readVarlinkMessage(reader io.Reader) ([]byte, error) {
	message := make([]byte, 0, 512)
	buffer := make([]byte, 4096)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			chunk := buffer[:n]
			if terminator := bytes.IndexByte(chunk, 0); terminator >= 0 {
				if len(message)+terminator > maxVarlinkMessageSize {
					return nil, fmt.Errorf("Varlink response exceeds %d bytes", maxVarlinkMessageSize)
				}
				message = append(message, chunk[:terminator]...)
				if len(chunk[terminator+1:]) > 0 {
					return nil, fmt.Errorf("Varlink response contains trailing data")
				}
				if len(message) == 0 {
					return nil, fmt.Errorf("Varlink response is empty")
				}
				return message, nil
			}
			if len(message)+len(chunk) > maxVarlinkMessageSize {
				return nil, fmt.Errorf("Varlink response exceeds %d bytes", maxVarlinkMessageSize)
			}
			message = append(message, chunk...)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("Varlink response ended without a terminator")
			}
			return nil, err
		}
	}
}

func validVarlinkIdentifier(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for _, char := range []byte(value) {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '.' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func contextIOError(ctx context.Context, operation string, err error) error {
	if contextErr := contextNetworkError(ctx, err); contextErr != nil {
		return contextErr
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func contextNetworkError(ctx context.Context, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		if deadline, hasDeadline := ctx.Deadline(); hasDeadline && !time.Now().Before(deadline) {
			return context.DeadlineExceeded
		}
	}
	return nil
}

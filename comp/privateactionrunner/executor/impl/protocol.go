// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

// Package impl — binary IPC protocol between par-control and par-executor.
//
// One connection per request (same model as the previous HTTP approach).
// Each connection carries exactly one frame in each direction.
//
// Request (par-control → par-executor):
//
//	[1]  frame_type   0x01 = ping  0x02 = execute
//	If execute:
//	  [4]  task_len    (LE uint32) length of raw task bytes
//	  [N]  task bytes  verbatim OPMS JSON — no base64 encoding
//	  [4]  timeout     (LE uint32) task timeout in seconds
//
// Response (par-executor → par-control):
//
//	If ping:   [1] 0x01 (pong)
//	If execute:
//	  [1]  status      0x00 = ok  0x01 = error
//	  [4]  payload_len (LE uint32)
//	  [N]  payload     JSON
//	       ok    → raw action output JSON bytes
//	       error → {"error_code": N, "error_details": "..."}
//
// This replaces the previous HTTP/1.1 + JSON + base64 transport.
// For a 15 MB task payload, the old approach allocated ~35 MB (base64 encode
// + JSON decode + base64 decode).  The binary protocol allocates exactly one
// buffer of task_len bytes — same as the original single-process PAR.
package impl

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

const (
	// Frame types — first byte sent by par-control on each new connection.
	framePing    byte = 0x01
	frameExecute byte = 0x02

	// Response status — first byte of an execute response.
	statusOK  byte = 0x00
	statusErr byte = 0x01
)

// readExecuteRequest reads the task bytes and timeout from a connection whose
// frame type has already been consumed.
func readExecuteRequest(conn net.Conn) (rawTask []byte, timeoutSecs uint32, err error) {
	var taskLen uint32
	if err = binary.Read(conn, binary.LittleEndian, &taskLen); err != nil {
		return nil, 0, fmt.Errorf("reading task_len: %w", err)
	}
	if taskLen == 0 {
		return nil, 0, fmt.Errorf("task_len is zero")
	}

	rawTask = make([]byte, taskLen)
	if _, err = io.ReadFull(conn, rawTask); err != nil {
		return nil, 0, fmt.Errorf("reading task bytes: %w", err)
	}

	if err = binary.Read(conn, binary.LittleEndian, &timeoutSecs); err != nil {
		return nil, 0, fmt.Errorf("reading timeout: %w", err)
	}
	return rawTask, timeoutSecs, nil
}

// writeOKResponse writes a success response with the given JSON payload.
func writeOKResponse(conn net.Conn, payload []byte) error {
	if _, err := conn.Write([]byte{statusOK}); err != nil {
		return err
	}
	if err := binary.Write(conn, binary.LittleEndian, uint32(len(payload))); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

// writeErrorResponse writes an error response with the given JSON payload.
func writeErrorResponse(conn net.Conn, payload []byte) error {
	if _, err := conn.Write([]byte{statusErr}); err != nil {
		return err
	}
	if err := binary.Write(conn, binary.LittleEndian, uint32(len(payload))); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

// readResponse reads a full execute response from a connection.
func readResponse(conn net.Conn) (status byte, payload []byte, err error) {
	var statusBuf [1]byte
	if _, err = io.ReadFull(conn, statusBuf[:]); err != nil {
		return 0, nil, fmt.Errorf("reading status: %w", err)
	}
	status = statusBuf[0]

	var payloadLen uint32
	if err = binary.Read(conn, binary.LittleEndian, &payloadLen); err != nil {
		return 0, nil, fmt.Errorf("reading payload_len: %w", err)
	}

	payload = make([]byte, payloadLen)
	if _, err = io.ReadFull(conn, payload); err != nil {
		return 0, nil, fmt.Errorf("reading payload: %w", err)
	}
	return status, payload, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type elapsedDeadlineContext struct {
	context.Context
}

func (elapsedDeadlineContext) Deadline() (time.Time, bool) {
	return time.Now().Add(-time.Second), true
}

func serveVarlinkResponse(conn net.Conn, response []byte, gate <-chan struct{}) (<-chan []byte, <-chan error) {
	request := make(chan []byte, 1)
	finished := make(chan error, 1)
	go func() {
		defer conn.Close()
		message, err := readVarlinkMessage(conn)
		if err != nil {
			finished <- err
			return
		}
		request <- message
		if gate != nil {
			<-gate
		}
		response = append(append([]byte(nil), response...), 0)
		finished <- writeAll(conn, response)
	}()
	return request, finished
}

func TestRotateJournalControlUsesFixedSynchronousMethod(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	gate := make(chan struct{})
	request, finished := serveVarlinkResponse(server, []byte(`{"parameters":{}}`), gate)

	rotationDone := make(chan error, 1)
	go func() {
		rotationDone <- callJournalRotateVarlink(context.Background(), client)
	}()

	var decoded varlinkRequest
	require.NoError(t, json.Unmarshal(<-request, &decoded))
	assert.Equal(t, journalRotateMethod, decoded.Method)
	select {
	case err := <-rotationDone:
		t.Fatalf("rotation returned before journald replied: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(gate)
	require.NoError(t, <-rotationDone)
	require.NoError(t, <-finished)
}

func TestRotateJournalControlReportsSafeProtocolErrors(t *testing.T) {
	for _, test := range []struct {
		name     string
		response string
		needle   string
	}{
		{name: "daemon error", response: `{"error":"io.systemd.Journal.NotSupported","parameters":{}}`, needle: "io.systemd.Journal.NotSupported"},
		{name: "unsafe daemon error", response: "{\"error\":\"bad\\nterminal\"}", needle: "invalid protocol error"},
		{name: "streaming", response: `{"parameters":{},"continues":true}`, needle: "streaming response"},
		{name: "unexpected parameters", response: `{"parameters":{"path":"/host"}}`, needle: "unexpected response parameters"},
		{name: "malformed", response: `[]`, needle: "malformed response"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			_, finished := serveVarlinkResponse(server, []byte(test.response), nil)

			err := callJournalRotateVarlink(context.Background(), client)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.needle)
			if test.name == "unsafe daemon error" {
				assert.NotContains(t, err.Error(), "terminal")
			}
			require.NoError(t, <-finished)
		})
	}
}

func TestContextNetworkErrorHandlesDeadlinePublicationRace(t *testing.T) {
	// Model the brief interval where the socket deadline has fired but the
	// context timer has not yet published context.DeadlineExceeded.
	ctx := elapsedDeadlineContext{Context: context.Background()}
	err := fmt.Errorf("wrapped timeout: %w", os.ErrDeadlineExceeded)

	assert.ErrorIs(t, contextNetworkError(ctx, err), context.DeadlineExceeded)
}

func TestRotateJournalControlHonorsCancellation(t *testing.T) {
	client, server := net.Pipe()
	gate := make(chan struct{})
	_, finished := serveVarlinkResponse(server, []byte(`{"parameters":{}}`), gate)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := callJournalRotateVarlink(ctx, client)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, client.Close())
	close(gate)
	serverErr := <-finished
	if serverErr != nil {
		assert.True(t, errors.Is(serverErr, net.ErrClosed) || errors.Is(serverErr, io.ErrClosedPipe) || strings.Contains(serverErr.Error(), "broken pipe"), serverErr.Error())
	}
}

func TestRotateJournalControlRejectsNonSocketEndpoints(t *testing.T) {
	client := &Client{}
	regular := filepath.Join(t.TempDir(), "journal.sock")
	require.NoError(t, os.WriteFile(regular, []byte("not a socket"), 0o600))
	err := client.rotateJournalControl(context.Background(), regular)
	assert.ErrorContains(t, err, "not a Unix socket")

	if runtime.GOOS == "windows" {
		return
	}
	symlink := filepath.Join(t.TempDir(), "journal-link.sock")
	require.NoError(t, os.Symlink(regular, symlink))
	err = client.rotateJournalControl(context.Background(), symlink)
	assert.ErrorContains(t, err, "not a Unix socket")
}

func TestReadVarlinkMessageBoundsResponses(t *testing.T) {
	_, err := readVarlinkMessage(strings.NewReader(strings.Repeat("x", maxVarlinkMessageSize+1) + "\x00"))
	assert.ErrorContains(t, err, "exceeds")

	_, err = readVarlinkMessage(strings.NewReader("{}"))
	assert.ErrorContains(t, err, "without a terminator")

	_, err = readVarlinkMessage(strings.NewReader("{}\x00trailing"))
	assert.ErrorContains(t, err, "trailing data")
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"testing"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/fakeintake/server/serverstore"
)

func TestTerminalExportJoinsAcceptedBodyAndIncludesEveryPage(t *testing.T) {
	ready := make(chan bool, 1)
	active := make(chan struct{})
	stopping := make(chan struct{})
	mock := clock.NewMock()
	fi := NewServer(WithAddress("127.0.0.1:0"), WithReadyChannel(ready), WithClock(mock))
	var once sync.Once
	fi.server.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateActive {
			once.Do(func() { close(active) })
		}
	}
	fi.server.RegisterOnShutdown(func() { close(stopping) })
	fi.Start()
	require.True(t, <-ready)
	for index := 0; index <= serverstore.MaxPageSize; index++ {
		require.NoError(t, fi.store.AppendPayload("/earlier", "key", []byte(strconv.Itoa(index)), "", "text/plain", mock.Now()))
	}
	reader, writer := io.Pipe()
	defer writer.Close()
	response := make(chan error, 1)
	go func() {
		request, err := http.NewRequest(http.MethodPost, fi.URL()+"/late", reader)
		if err != nil {
			response <- err
			return
		}
		request.Header.Add("X-Terminal-Test", "first")
		request.Header.Add("X-Terminal-Test", "second")
		reply, err := http.DefaultClient.Do(request)
		if err == nil {
			_, readErr := io.Copy(io.Discard, reply.Body)
			err = errors.Join(readErr, reply.Body.Close())
		}
		response <- err
	}()
	<-active
	var output bytes.Buffer
	finished := make(chan error, 1)
	go func() { finished <- fi.StopAndExport(&output) }()
	<-stopping
	select {
	case err := <-finished:
		t.Fatalf("export completed before the accepted body settled: %v", err)
	default:
	}
	_, writeErr := io.WriteString(writer, "late original body")
	closeErr := writer.Close()
	requestErr := <-response
	exportErr := <-finished
	require.NoError(t, errors.Join(writeErr, closeErr, requestErr, exportErr))
	var exported struct {
		Payloads map[string][]api.Payload `json:"payloads"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &exported))
	require.Len(t, exported.Payloads["/earlier"], serverstore.MaxPageSize+1)
	for index, payload := range exported.Payloads["/earlier"] {
		require.Equal(t, []byte(strconv.Itoa(index)), payload.Data)
	}
	require.Len(t, exported.Payloads["/late"], 1)
	late := exported.Payloads["/late"][0]
	require.Equal(t, []byte("late original body"), late.Data)
	require.NotNil(t, late.Request)
	require.Equal(t, []string{"first", "second"}, late.Request.Headers.Values("X-Terminal-Test"))
	select {
	case <-fi.cleanupDone:
	default:
		t.Fatal("retention worker was not joined")
	}
}

type failedExportWriter struct{}

func (failedExportWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestTerminalExportPropagatesWriterFailure(t *testing.T) {
	fi, _ := InitialiseForTests(t)
	require.ErrorIs(t, fi.StopAndExport(failedExportWriter{}), io.ErrClosedPipe)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package agentimpl

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	compressionmock "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

// capturingIntake is a minimal HTTP log intake that records the raw body of
// every request it receives.
type capturingIntake struct {
	server *httptest.Server

	mu     sync.Mutex
	bodies []string
}

func newCapturingIntake(t *testing.T) *capturingIntake {
	ci := &capturingIntake{}
	ci.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err == nil {
			ci.mu.Lock()
			ci.bodies = append(ci.bodies, string(body))
			ci.mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ci.server.Close)
	return ci
}

func (ci *capturingIntake) endpoint(t *testing.T) config.Endpoint {
	url := strings.TrimPrefix(ci.server.URL, "http://")
	parts := strings.SplitN(url, ":", 2)
	require.Len(t, parts, 2)
	port, err := strconv.Atoi(parts[1])
	require.NoError(t, err)
	return config.NewEndpoint("test", "", parts[0], port, config.EmptyPathPrefix, false)
}

func (ci *capturingIntake) count(substr string) int {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	n := 0
	for _, body := range ci.bodies {
		n += strings.Count(body, substr)
	}
	return n
}

// TestDrainTailersDeliversUnreadLine reproduces the Cloud Run cold-start gap:
// a line is written to the tailed file but the tailer never gets scheduled
// to read it before shutdown. DrainTailers must force that final read (via
// the file launcher's Stop) so the line reaches the pipeline before Flush
// ships it.
func TestDrainTailersDeliversUnreadLine(t *testing.T) {
	intake := newCapturingIntake(t)
	endpoints := config.NewEndpoints(intake.endpoint(t), nil, false, true)

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeCompression := compressionmock.NewMockCompressor()
	hostnameService := hostnameimpl.NewHostnameService()

	serverlessLogsAgent := NewServerlessLogsAgent(fakeTagger, fakeCompression, hostnameService)
	logsAgent, ok := serverlessLogsAgent.(*logAgent)
	require.True(t, ok, "Expected NewServerlessLogsAgent to return *logAgent type")

	logsAgent.endpoints = endpoints
	require.NoError(t, logsAgent.setupAgent())
	logsAgent.startPipeline()
	t.Cleanup(func() { _ = logsAgent.stop(context.Background()) })

	dir := t.TempDir()
	path := dir + "/cold-start.log"
	require.NoError(t, os.WriteFile(path, nil, 0o644))

	logConfig := config.LogsConfig{
		Type:        config.FileType,
		Path:        path,
		Identifier:  "cold-start-test",
		TailingMode: "end",
	}
	source := sources.NewLogSource("cold-start-test", &logConfig)
	logsAgent.sources.AddSource(source)

	// Wait for the file launcher to attach a tailer for this (empty, end-tailed)
	// file. Once attached, the tailer's readForever immediately sees EOF and
	// goes to sleep for filelauncher.DefaultSleepDuration before its next
	// read -- exactly the window the cold-start line lands in below.
	testutil.AssertTrueBeforeTimeout(t, 5*time.Millisecond, 2*time.Second, func() bool {
		return len(logsAgent.tracker.All()) == 1
	})

	const pendingLine = "cold-start-request-line"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString(pendingLine + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	logsAgent.DrainTailers(ctx)
	logsAgent.Flush(ctx)

	require.Equal(t, 1, intake.count(pendingLine))
}

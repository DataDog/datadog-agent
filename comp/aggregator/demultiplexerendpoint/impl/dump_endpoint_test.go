// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package demultiplexerendpointimpl

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type blockingContextDumper struct {
	started chan struct{}
	release chan struct{}
}

func (d *blockingContextDumper) DumpDogstatsdContexts(w io.Writer) error {
	d.started <- struct{}{}
	<-d.release
	_, err := io.WriteString(w, "{}\n")
	return err
}

func TestDumpDogstatsdContextsRejectsWhenDataPlaneOwnsDogstatsd(t *testing.T) {
	endpoint := demultiplexerEndpoint{dogstatsdOnDataPlane: true}
	recorder := httptest.NewRecorder()

	endpoint.dumpDogstatsdContexts(recorder, httptest.NewRequest(http.MethodPost, "/dogstatsd-contexts-dump", nil))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Agent Data Plane")
}

func TestWriteDogstatsdContextsSerializesConcurrentDumps(t *testing.T) {
	dumper := &blockingContextDumper{
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	endpoint := demultiplexerEndpoint{
		demux:   dumper,
		runPath: t.TempDir(),
		dumpMu:  &sync.Mutex{},
	}
	results := make(chan error, 2)

	for range 2 {
		go func() {
			_, err := endpoint.writeDogstatsdContexts()
			results <- err
		}()
	}

	<-dumper.started
	concurrent := false
	select {
	case <-dumper.started:
		concurrent = true
	case <-time.After(100 * time.Millisecond):
	}
	close(dumper.release)

	require.NoError(t, <-results)
	require.NoError(t, <-results)
	require.False(t, concurrent)
}

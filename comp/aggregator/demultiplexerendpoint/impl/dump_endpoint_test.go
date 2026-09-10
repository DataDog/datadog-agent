// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package demultiplexerendpointimpl

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type contextDumperFunc func(io.Writer) error

func (f contextDumperFunc) DumpDogstatsdContexts(w io.Writer) error {
	return f(w)
}

func TestDumpDogstatsdContextsRejectsWhenDataPlaneOwnsDogstatsd(t *testing.T) {
	endpoint := demultiplexerEndpoint{dogstatsdOnDataPlane: true}
	recorder := httptest.NewRecorder()

	endpoint.dumpDogstatsdContexts(recorder, httptest.NewRequest(http.MethodPost, "/dogstatsd-contexts-dump", nil))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Agent Data Plane")
}

func TestWriteDogstatsdContextsLocksDump(t *testing.T) {
	endpoint := demultiplexerEndpoint{runPath: t.TempDir()}
	lockHeld := false
	endpoint.demux = contextDumperFunc(func(w io.Writer) error {
		if endpoint.dumpMu.TryLock() {
			endpoint.dumpMu.Unlock()
		} else {
			lockHeld = true
		}
		_, err := io.WriteString(w, "{}\n")
		return err
	})

	_, err := endpoint.writeDogstatsdContexts()
	require.NoError(t, err)
	require.True(t, lockHeld)
}

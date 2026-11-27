// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || darwin

package helpers

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// formFileTransport reads the request until the archive's form part starts, so the upload goroutine goes on to open it.
type formFileTransport struct {
	err error
}

func (rt *formFileTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := readPastFormFileHeader(req.Body); err != nil {
		return nil, err
	}
	if rt.err != nil {
		return nil, rt.err
	}
	return &http.Response{
		StatusCode: http.StatusInternalServerError,
		Status:     "500 Internal Server Error",
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("Internal Server Error")),
	}, nil
}

func TestReadAndPostFlareFileReturnsWhenArchiveOpenBlocks(t *testing.T) {
	testCases := []struct {
		name         string
		transportErr error
	}{
		{name: "server error response"},
		{name: "transport error", transportErr: errors.New("connection reset")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Opening a FIFO for reading blocks until a writer opens it, as a read from an unresponsive filesystem would.
			archivePath := filepath.Join(t.TempDir(), "flare.zip")
			require.NoError(t, syscall.Mkfifo(archivePath, 0600))

			done := make(chan error, 1)
			go func() {
				client := &http.Client{Transport: &formFileTransport{err: tc.transportErr}, Timeout: 100 * time.Millisecond}
				resp, err := readAndPostFlareFile(archivePath, "12345", "", "hostname", "http://flare.invalid/support/flare/12345", FlareSource{}, client, "test-api-key")
				if resp != nil {
					resp.Body.Close()
				}
				done <- err
			}()

			var err error
			returned := false
			select {
			case err = <-done:
				returned = true
			case <-time.After(5 * time.Second):
				assert.Fail(t, "readAndPostFlareFile should return while the archive open is blocked")
			}

			// Let the upload goroutine finish its open, if it is in it.
			if writer, openErr := os.OpenFile(archivePath, os.O_WRONLY|syscall.O_NONBLOCK, 0); openErr == nil {
				writer.Close()
			}
			if !returned {
				select {
				case err = <-done:
				case <-time.After(5 * time.Second):
					require.FailNow(t, "readAndPostFlareFile should return once the archive open completes")
				}
			}
			if tc.transportErr != nil {
				// http.Client wraps the error in its timeout error when the timeout fires first.
				assert.ErrorContains(t, err, tc.transportErr.Error())
			} else {
				assert.ErrorContains(t, err, "HTTP 500")
			}
		})
	}
}

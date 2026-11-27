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

			// The upload goroutine opens the archive once the transport has read the form header, so a blocking
			// writer open returns once that goroutine is in its open, however late it gets there.
			type openResult struct {
				writer *os.File
				err    error
			}
			opened := make(chan openResult, 1)
			go func() {
				writer, openErr := os.OpenFile(archivePath, os.O_WRONLY, 0)
				opened <- openResult{writer, openErr}
			}()
			select {
			case res := <-opened:
				require.NoError(t, res.err)
				res.writer.Close()
			case <-time.After(5 * time.Second):
				// A reader of our own ends the blocked writer open. Unlink the FIFO before closing the writer so a
				// later open by the upload goroutine fails instead of waiting for another writer.
				if reader, readerErr := os.OpenFile(archivePath, os.O_RDONLY|syscall.O_NONBLOCK, 0); readerErr == nil {
					res := <-opened
					os.Remove(archivePath)
					if res.err == nil {
						res.writer.Close()
					}
					reader.Close()
				}
				require.FailNow(t, "the upload goroutine should open the archive")
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

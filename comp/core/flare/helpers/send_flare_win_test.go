// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package helpers

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func assertArchiveClosed(t *testing.T, path string) {
	t.Helper()
	p, err := windows.UTF16PtrFromString(extendedLengthPath(path))
	require.NoError(t, err)
	// An open with no sharing fails while any other handle on the file is open; retry briefly in case a scanner holds one.
	var h windows.Handle
	for range 20 {
		h, err = windows.CreateFile(p, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if err != windows.ERROR_SHARING_VIOLATION {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if assert.NoError(t, err, "the archive should be closed once the upload is over") {
		windows.CloseHandle(h) //nolint:errcheck
	}
}

func TestExtendedLengthPath(t *testing.T) {
	long := `C:\` + strings.Repeat(`d\`, 130) + "flare.zip"
	assert.Equal(t, `C:\Temp\flare.zip`, extendedLengthPath(`C:\Temp\flare.zip`))
	assert.Equal(t, `\\?\`+long, extendedLengthPath(long))
	assert.Equal(t, `\\?\`+long, extendedLengthPath(`\\?\`+long))
	assert.Equal(t, `\\?\`+long, extendedLengthPath(strings.ReplaceAll(long, `\`, "/")))
	unc := `\\server\share\` + strings.Repeat(`d\`, 130) + "flare.zip"
	assert.Equal(t, `\\?\UNC\`+unc[2:], extendedLengthPath(unc))
}

func TestReadAndPostFlareFileUploadsArchiveBeyondMaxPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), strings.Repeat("d", 100), strings.Repeat("e", 100), strings.Repeat("f", 100))
	require.NoError(t, os.MkdirAll(dir, 0700))
	archivePath := filepath.Join(dir, "flare.zip")
	require.Greater(t, len(archivePath), 260)
	require.NoError(t, os.WriteFile(archivePath, make([]byte, 1<<20), 0600))

	rt := &lateCloseTransport{}
	resp, err := readAndPostFlareFile(archivePath, "12345", "test@example.com", "hostname", "http://flare.invalid/support/flare/12345", FlareSource{}, &http.Client{Transport: rt}, "test-api-key")
	require.NoError(t, err)
	resp.Body.Close()
	rt.body.Close()
	assertArchiveClosed(t, archivePath)
}

func TestOpenArchiveSupportsReadDeadline(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "flare.zip")
	require.NoError(t, os.WriteFile(archivePath, []byte("archive"), 0600))

	file, err := openArchive(archivePath)
	require.NoError(t, err)
	defer file.Close()
	assert.NoError(t, file.SetReadDeadline(time.Now().Add(time.Minute)), "the archive handle should accept a read deadline")
}

func TestFlareReaderEndsBlockedArchiveReadAtDeadline(t *testing.T) {
	// A pipe whose server never writes blocks the archive read, as an unresponsive filesystem would.
	name := `\\.\pipe\datadog-flare-test-` + strconv.Itoa(os.Getpid())
	namep, err := windows.UTF16PtrFromString(name)
	require.NoError(t, err)
	server, err := windows.CreateNamedPipe(namep, windows.PIPE_ACCESS_OUTBOUND, windows.PIPE_TYPE_BYTE|windows.PIPE_WAIT, 1, 4096, 4096, 0, nil)
	require.NoError(t, err)
	defer windows.CloseHandle(server) //nolint:errcheck

	body := getFlareReader("boundary", name, "12345", "", "hostname", FlareSource{}, time.Now().Add(100*time.Millisecond))
	defer body.release()
	require.NoError(t, readPastFormFileHeader(body))

	readErr := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, body)
		readErr <- err
	}()
	select {
	case err := <-readErr:
		assert.ErrorIs(t, err, os.ErrDeadlineExceeded)
	case <-time.After(5 * time.Second):
		assert.Fail(t, "the archive read should end at the upload deadline")
	}
}

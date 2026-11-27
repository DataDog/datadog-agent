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

func TestExtendedLengthPath(t *testing.T) {
	long := `C:\` + strings.Repeat(`d\`, 130) + "flare.zip"
	assert.Equal(t, `C:\Temp\flare.zip`, extendedLengthPath(`C:\Temp\flare.zip`))
	assert.Equal(t, `\\?\`+long, extendedLengthPath(long))
	assert.Equal(t, `\\?\`+long, extendedLengthPath(`\\?\`+long))
	assert.Equal(t, `\\?\`+long, extendedLengthPath(strings.ReplaceAll(long, `\`, "/")))
	unc := `\\server\share\` + strings.Repeat(`d\`, 130) + "flare.zip"
	assert.Equal(t, `\\?\UNC\`+unc[2:], extendedLengthPath(unc))
	device := `//./C:/` + strings.Repeat(`d/`, 130) + "flare.zip"
	assert.Equal(t, device, extendedLengthPath(device))
	assert.Equal(t, `\??\`+long, extendedLengthPath(`\??\`+long))
}

func TestExtendedLengthPathResolvesRelativePath(t *testing.T) {
	dir := t.TempDir()
	if len(dir) > 200 {
		t.Skipf("temp dir %q leaves no room for a relative path that is short on its own", dir)
	}
	t.Chdir(dir)
	// Short on its own, but at least 248 characters once joined with the working directory.
	rel := strings.Repeat("a", 248-len(dir))
	assert.Equal(t, `\\?\`+filepath.Join(dir, rel), extendedLengthPath(rel))
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
	assertUploadOver(t, rt)
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
	name := `\\.\pipe\datadog-flare-test-` + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	namep, err := windows.UTF16PtrFromString(name)
	require.NoError(t, err)
	server, err := windows.CreateNamedPipe(namep, windows.PIPE_ACCESS_OUTBOUND, windows.PIPE_TYPE_BYTE|windows.PIPE_WAIT, 1, 4096, 4096, 0, nil)
	require.NoError(t, err)
	defer windows.CloseHandle(server) //nolint:errcheck

	body := getFlareReader("boundary", name, "12345", "", "hostname", FlareSource{}, time.Now().Add(100*time.Millisecond))
	defer func() {
		body.release()
		// release stops waiting at the deadline; wait for the upload goroutine to close its pipe handle too.
		select {
		case <-body.done:
		case <-time.After(5 * time.Second):
			assert.Fail(t, "the upload goroutine should close the archive")
		}
	}()
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

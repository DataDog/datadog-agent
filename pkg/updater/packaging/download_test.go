// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packaging

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"

	"github.com/mholt/archiver/v3"
	"github.com/stretchr/testify/assert"
)

const (
	testAgentFileName        = "agent"
	testAgentArchiveFileName = "agent.tar.gz"
	testDownloadDir          = "download"
)

func createTestArchive(t *testing.T, dir string) {
	filePath := path.Join(dir, testAgentFileName)
	err := os.WriteFile(filePath, []byte("test"), 0644)
	assert.NoError(t, err)
	archivePath := path.Join(dir, testAgentArchiveFileName)
	err = archiver.DefaultTarGz.Archive([]string{filePath}, archivePath)
	assert.NoError(t, err)
}

func createTestServer(t *testing.T, dir string) *httptest.Server {
	createTestArchive(t, dir)
	return httptest.NewServer(http.FileServer(http.Dir(dir)))
}

func agentArchiveHash(t *testing.T, dir string) []byte {
	f, err := os.Open(path.Join(dir, testAgentArchiveFileName))
	assert.NoError(t, err)
	defer f.Close()
	hash := sha256.New()
	_, err = io.Copy(hash, f)
	assert.NoError(t, err)
	return hash.Sum(nil)
}

func TestDownload(t *testing.T) {
	dir := t.TempDir()
	server := createTestServer(t, dir)
	defer server.Close()
	downloader := NewDownloader(server.Client())
	downloadPath := path.Join(dir, testDownloadDir)
	err := os.MkdirAll(downloadPath, 0755)
	assert.NoError(t, err)

	pkg := RemotePackage{URL: fmt.Sprintf("%s/%s", server.URL, testAgentArchiveFileName), SHA256: agentArchiveHash(t, dir)}
	err = downloader.Download(context.Background(), pkg, downloadPath)
	assert.NoError(t, err)
	assert.FileExists(t, path.Join(downloadPath, testAgentFileName))
}

func TestDownloadCheckHash(t *testing.T) {
	dir := t.TempDir()
	server := createTestServer(t, dir)
	defer server.Close()
	downloader := NewDownloader(server.Client())
	downloadPath := path.Join(dir, testDownloadDir)
	err := os.MkdirAll(downloadPath, 0755)
	assert.NoError(t, err)

	fakeHash := sha256.Sum256([]byte(`test`))
	pkg := RemotePackage{URL: fmt.Sprintf("%s/%s", server.URL, testAgentArchiveFileName), SHA256: fakeHash[:]}
	err = downloader.Download(context.Background(), pkg, downloadPath)
	assert.Error(t, err)
}

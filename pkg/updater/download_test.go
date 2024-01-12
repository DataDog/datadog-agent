// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	testAgentFileName        = "agent"
	testNestedAgentFileName  = "nested/agent2"
	testAgentArchiveFileName = "agent.tar.gz"
	testDownloadDir          = "download"
)

func createTestArchive(t *testing.T, dir string) {
	filePath := path.Join(dir, testAgentFileName)
	err := os.WriteFile(filePath, []byte("test"), 0644)
	assert.NoError(t, err)
	nestedFilePath := path.Join(dir, testNestedAgentFileName)
	err = os.MkdirAll(path.Dir(nestedFilePath), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(nestedFilePath, []byte("test"), 0644)
	assert.NoError(t, err)
	archivePath := path.Join(dir, testAgentArchiveFileName)
	files := []string{testAgentFileName, testNestedAgentFileName}
	out, err := os.Create(archivePath)
	assert.NoError(t, err)
	defer out.Close()
	err = createArchive(dir, files, out)
	assert.NoError(t, err)
}

func createTestServer(t *testing.T, dir string) *httptest.Server {
	createTestArchive(t, dir)
	return httptest.NewServer(http.FileServer(http.Dir(dir)))
}

func agentArchiveHash(t *testing.T, dir string) string {
	f, err := os.Open(path.Join(dir, testAgentArchiveFileName))
	assert.NoError(t, err)
	defer f.Close()
	hash := sha256.New()
	_, err = io.Copy(hash, f)
	assert.NoError(t, err)
	return hex.EncodeToString(hash.Sum(nil))
}

func createArchive(dir string, files []string, buf io.Writer) error {
	// Create new Writers for gzip and tar
	// These writers are chained. Writing to the tar writer will
	// write to the gzip writer which in turn will write to
	// the "buf" writer
	gw := gzip.NewWriter(buf)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Iterate over files and add them to the tar archive
	for _, file := range files {
		err := addToArchive(dir, tw, file)
		if err != nil {
			return err
		}
	}

	return nil
}

func addToArchive(dir string, tw *tar.Writer, filename string) error {
	file, err := os.Open(filepath.Join(dir, filename))
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(info, info.Name())
	if err != nil {
		return err
	}

	header.Name = filename

	err = tw.WriteHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(tw, file)
	if err != nil {
		return err
	}

	return nil
}

func TestDownload(t *testing.T) {
	dir := t.TempDir()
	server := createTestServer(t, dir)
	defer server.Close()
	downloader := newDownloader(server.Client())
	downloadPath := path.Join(dir, testDownloadDir)
	err := os.MkdirAll(downloadPath, 0755)
	assert.NoError(t, err)

	pkg := Package{URL: fmt.Sprintf("%s/%s", server.URL, testAgentArchiveFileName), SHA256: agentArchiveHash(t, dir)}
	err = downloader.Download(context.Background(), pkg, downloadPath)
	assert.NoError(t, err)
	assert.FileExists(t, path.Join(downloadPath, testAgentFileName))
	assert.FileExists(t, path.Join(downloadPath, testNestedAgentFileName))
}

func TestDownloadCheckHash(t *testing.T) {
	dir := t.TempDir()
	server := createTestServer(t, dir)
	defer server.Close()
	downloader := newDownloader(server.Client())
	downloadPath := path.Join(dir, testDownloadDir)
	err := os.MkdirAll(downloadPath, 0755)
	assert.NoError(t, err)

	fakeHash := sha256.Sum256([]byte(`test`))
	pkg := Package{URL: fmt.Sprintf("%s/%s", server.URL, testAgentArchiveFileName), SHA256: hex.EncodeToString(fakeHash[:])}
	err = downloader.Download(context.Background(), pkg, downloadPath)
	assert.Error(t, err)
}

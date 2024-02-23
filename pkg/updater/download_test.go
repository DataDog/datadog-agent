// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

const (
	agentPrefix              = "opt/datadog-packages/datadog-agent/7.51.0/"
	testAgentFileName        = "agent"
	testNestedAgentFileName  = "nested/agent2"
	testLargeFileName        = "large"
	testLargeFileSize        = 1024 * 1024 * 20 // 20MB
	testAgentArchiveFileName = "package.tar.gz"
	testDownloadDir          = "download"
)

type DownloadTestSuite struct {
	suite.Suite
	dir    string
	server *httptest.Server
}

func (suite *DownloadTestSuite) SetupTest() {
	suite.dir = suite.T().TempDir()
	suite.server = httptest.NewServer(http.FileServer(http.Dir(suite.dir)))
	createTestOCIArchive(suite.T(), suite.dir)
}

func (suite *DownloadTestSuite) TearDownSuite() {
	suite.server.Close()
}

func TestDownloadTestSuite(t *testing.T) {
	suite.Run(t, new(DownloadTestSuite))
}

// createTestOCIArchive creates a minimal OCI test archive
func createTestOCIArchive(t *testing.T, dir string) {
	blobPath := path.Join(dir, "blobs/sha256")

	err := os.MkdirAll(blobPath, 0755)
	assert.NoError(t, err)

	// Layer: tar.gz archive containing the actual files
	createTestArchive(t, blobPath, agentPrefix, "layer")
	layerPath := path.Join(blobPath, "layer")

	// Calculate size & digest after writing
	hasher := sha256.New()
	s, err := os.Open(layerPath)
	if err != nil {
		log.Fatal(err)
	}
	_, err = io.Copy(hasher, s)
	if err != nil {
		log.Fatal(err)
	}
	layerDigest := hex.EncodeToString(hasher.Sum(nil))
	layerDigestPath := path.Join(blobPath, layerDigest)
	// File names are digests: move file
	err = os.Rename(layerPath, layerDigestPath)
	assert.NoError(t, err)
	layerStat, err := os.Stat(layerDigestPath)
	assert.NoError(t, err)

	// Manifest: data about the layer
	manifestPath := path.Join(dir, "blobs/sha256", "manifest")
	err = os.WriteFile(
		manifestPath,
		[]byte(fmt.Sprintf(
			`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","artifactType":"application/vnd.datadoghq.pkg","config":{"mediaType":"application/vnd.datadoghq.pkgmetadata.v1+json","digest":"sha256:%[1]s","size":%[2]d},"layers":[{"mediaType":"application/vnd.oci.image.layer.v1.tar+gzip","digest":"sha256:%[1]s","size":%[2]d}]}`,
			layerDigest, layerStat.Size(),
		),
		),
		0o755,
	)
	assert.NoError(t, err)

	// Calculate size & digest after writing
	hasher = sha256.New()
	s, err = os.Open(manifestPath)
	if err != nil {
		log.Fatal(err)
	}
	_, err = io.Copy(hasher, s)
	if err != nil {
		log.Fatal(err)
	}
	manifestDigest := hex.EncodeToString(hasher.Sum(nil))
	manifestDigestPath := path.Join(blobPath, manifestDigest)
	// File names are digests: move file
	os.Rename(manifestPath, manifestDigestPath)
	manifestStat, err := os.Stat(manifestDigestPath)
	assert.NoError(t, err)

	// index.json: the root of the OCI archive
	indexPath := path.Join(dir, "index.json")
	err = os.WriteFile(
		indexPath,
		[]byte(fmt.Sprintf(
			`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","size":%d,"digest":"sha256:%s","platform":{"architecture":"amd64","os":"linux"}}],"annotations":{"com.datadoghq.package.name":"datadog-agent","com.datadoghq.package.version":"7.52.0-devel.git.513.0ad807a.pipeline.28042376-1","com.datadoghq.package.license":"Apache-2.0"}}`,
			manifestStat.Size(),
			manifestDigest,
		),
		),
		0o755,
	)
	assert.NoError(t, err)

	// Pack the OCI archive
	archivePath := path.Join(dir, testAgentArchiveFileName)
	out, err := os.Create(archivePath)
	assert.NoError(t, err)
	defer out.Close()

	files := []string{
		"index.json",
		path.Join("blobs/sha256", manifestDigest),
		path.Join("blobs/sha256", layerDigest),
	}
	err = createArchive(dir, files, out, compressionNone)
	assert.NoError(t, err)

	// Remove temporary files used for archive creation
	os.RemoveAll(path.Join(dir, "blobs"))
	os.RemoveAll(indexPath)
}

func createTestArchive(t *testing.T, dir string, filesPrefix string, archiveFilename string) {
	nestedFilePath := path.Join(dir, filesPrefix, testNestedAgentFileName)
	err := os.MkdirAll(path.Dir(nestedFilePath), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(nestedFilePath, []byte("test"), 0644)
	assert.NoError(t, err)

	agentFilePath := path.Join(dir, filesPrefix, testAgentFileName)
	err = os.WriteFile(agentFilePath, []byte("test"), 0644)
	assert.NoError(t, err)

	largeFilePath := path.Join(dir, filesPrefix, testLargeFileName)
	largeFile, err := os.Create(largeFilePath)
	assert.NoError(t, err)
	defer largeFile.Close()
	_, err = io.CopyN(largeFile, rand.Reader, testLargeFileSize)
	assert.NoError(t, err)

	archivePath := path.Join(dir, archiveFilename)
	files := []string{
		path.Join(filesPrefix, testAgentFileName),
		path.Join(filesPrefix, testNestedAgentFileName),
		path.Join(filesPrefix, testLargeFileName),
	}

	out, err := os.Create(archivePath)
	assert.NoError(t, err)
	defer out.Close()
	err = createArchive(dir, files, out, compressionGzip)
	assert.NoError(t, err)

	os.Remove(agentFilePath)
	os.RemoveAll(path.Dir(nestedFilePath))
	os.Remove(largeFilePath)
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

func agentArchiveSize(t *testing.T, dir string) int64 {
	f, err := os.Stat(path.Join(dir, testAgentArchiveFileName))
	assert.NoError(t, err)
	return int64(f.Size())
}

func createArchive(dir string, files []string, buf io.Writer, compression compression) error {
	// Create new Writers for gzip and tar
	// These writers are chained. Writing to the tar writer will
	// write to the gzip writer which in turn will write to
	// the "buf" writer
	var w = buf
	switch compression {
	case compressionGzip:
		gzw := gzip.NewWriter(buf)
		defer gzw.Close()
		w = gzw
	}
	tw := tar.NewWriter(w)
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
	file, err := os.Open(path.Join(dir, filename))
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

func (suite *DownloadTestSuite) TestDownload() {
	t := suite.T()
	downloader := newDownloader(suite.server.Client())
	downloadPath := path.Join(suite.dir, testDownloadDir)

	pkg := Package{
		Name:    "datadog-agent",
		Version: "7.51.0",
		URL:     fmt.Sprintf("%s/%s", suite.server.URL, testAgentArchiveFileName),
		SHA256:  agentArchiveHash(t, suite.dir),
		Size:    agentArchiveSize(t, suite.dir),
	}
	err := downloader.Download(context.Background(), pkg, downloadPath)

	// Read contents of downloadPath
	assert.NoError(t, err)
	assert.FileExists(t, path.Join(downloadPath, testAgentFileName))
	assert.FileExists(t, path.Join(downloadPath, testNestedAgentFileName))
	assert.FileExists(t, path.Join(downloadPath, testLargeFileName))

	// ensures the full archive or full individual files are not loaded in memory
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	assert.Less(t, m.TotalAlloc, uint64(testLargeFileSize))
}

func (suite *DownloadTestSuite) TestDownloadCheckHash() {
	t := suite.T()
	downloader := newDownloader(suite.server.Client())
	downloadPath := path.Join(suite.dir, testDownloadDir)
	err := os.MkdirAll(downloadPath, 0755)
	assert.NoError(t, err)

	fakeHash := sha256.Sum256([]byte(`test`))
	pkg := Package{
		Name:    "datadog-agent",
		Version: "7.51.0",
		URL:     fmt.Sprintf("%s/%s", suite.server.URL, testAgentArchiveFileName),
		SHA256:  hex.EncodeToString(fakeHash[:]),
		Size:    agentArchiveSize(t, suite.dir),
	}
	err = downloader.Download(context.Background(), pkg, downloadPath)
	assert.Error(t, err)
}

func (suite *DownloadTestSuite) TestDownloadCheckSize() {
	t := suite.T()
	downloader := newDownloader(suite.server.Client())
	downloadPath := path.Join(suite.dir, testDownloadDir)
	err := os.MkdirAll(downloadPath, 0755)
	assert.NoError(t, err)

	pkg := Package{
		Name:    "datadog-agent",
		Version: "7.51.0",
		URL:     fmt.Sprintf("%s/%s", suite.server.URL, testAgentArchiveFileName),
		SHA256:  agentArchiveHash(t, suite.dir),
		Size:    12345,
	}
	err = downloader.Download(context.Background(), pkg, downloadPath)
	assert.Error(t, err)
}

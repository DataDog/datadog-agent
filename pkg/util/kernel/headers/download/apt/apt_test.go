// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package apt

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	aptlyhttp "github.com/aptly-dev/aptly/http"
	"github.com/aptly-dev/aptly/utils"
	"github.com/stretchr/testify/require"
	"github.com/xor-gate/ar"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

type quietLogger struct{ types.Logger }

func (quietLogger) Debugf(string, ...interface{}) {}

func headerPackage(t *testing.T) []byte {
	t.Helper()
	var payload bytes.Buffer
	gz := gzip.NewWriter(&payload)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "types.h", Mode: 0644, Size: 6}))
	_, err := tw.Write([]byte("header"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	var pkg bytes.Buffer
	aw := ar.NewWriter(&pkg)
	require.NoError(t, aw.WriteGlobalHeader())
	require.NoError(t, aw.WriteHeader(&ar.Header{Name: "data.tar.gz", Size: int64(payload.Len()), Mode: 0644}))
	_, err = aw.Write(payload.Bytes())
	require.NoError(t, err)
	return pkg.Bytes()
}

func TestDownloadAuthenticatesBeforeExtraction(t *testing.T) {
	pkg := headerPackage(t)
	good := utils.ChecksumInfo{Size: int64(len(pkg)), SHA256: fmt.Sprintf("%x", sha256.Sum256(pkg))}
	for _, tc := range []struct {
		name    string
		sum     utils.ChecksumInfo
		content []byte
		wantOK  bool
	}{
		{"http sha256", good, pkg, true},
		{"http sha512", utils.ChecksumInfo{Size: int64(len(pkg)), SHA512: fmt.Sprintf("%x", sha512.Sum512(pkg))}, pkg, true},
		{"substituted bytes", good, bytes.Repeat([]byte{'x'}, len(pkg)), false},
		{"wrong size", utils.ChecksumInfo{Size: 1, SHA256: good.SHA256}, pkg, false},
		{"missing hash", utils.ChecksumInfo{Size: int64(len(pkg))}, pkg, false},
		{"weak hash only", utils.ChecksumInfo{Size: int64(len(pkg)), SHA1: "1234"}, pkg, false},
		{"malformed hash", utils.ChecksumInfo{Size: int64(len(pkg)), SHA256: "bad"}, pkg, false},
		{"invalid archive", utils.ChecksumInfo{Size: 3, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("bad")))}, []byte("bad"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tc.content) }))
			defer server.Close()
			dir := t.TempDir()
			b := Backend{logger: quietLogger{}}
			err := b.downloadAndExtract(aptlyhttp.NewDownloader(0, 1, nil), server.URL+"/headers.deb?download=1", tc.sum, dir)
			if tc.wantOK {
				require.NoError(t, err)
				content, e := os.ReadFile(filepath.Join(dir, "types.h"))
				require.NoError(t, e)
				require.Equal(t, "header", string(content))
			} else {
				require.Error(t, err)
				require.NoFileExists(t, filepath.Join(dir, "types.h"))
			}
			entries, e := os.ReadDir(dir)
			require.NoError(t, e)
			for _, entry := range entries {
				require.Equal(t, "types.h", entry.Name(), "download artifacts must be removed")
			}
		})
	}
}

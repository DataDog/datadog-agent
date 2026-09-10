// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !ebpf_bindata

package bytecode

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAssetContent is the plaintext behind testdata/asset.xz. That fixture is
// committed because github.com/xi2/xz is a decoder only, so the test cannot
// produce an xz stream itself. To regenerate it:
//
//	python3 -c "open('asset','wb').write(b''.join(b'ebpf asset line %05d\n' % i for i in range(500)))"
//	xz -9 -c asset > pkg/ebpf/bytecode/testdata/asset.xz
func testAssetContent() []byte {
	var buf bytes.Buffer
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&buf, "ebpf asset line %05d\n", i)
	}
	return buf.Bytes()
}

func gzipBytes(t *testing.T, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func TestDecompressAssetGzip(t *testing.T) {
	content := testAssetContent()

	reader, err := decompressAsset(".gz", bytes.NewReader(gzipBytes(t, content)), maxDecompressedAssetSize)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, reader.Close()) })

	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestDecompressAssetXZ(t *testing.T) {
	compressed, err := os.ReadFile(filepath.Join("testdata", "asset.xz"))
	require.NoError(t, err)

	reader, err := decompressAsset(".xz", bytes.NewReader(compressed), maxDecompressedAssetSize)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, reader.Close()) })

	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, testAssetContent(), got)
}

// TestDecompressAssetRandomAccess covers the reason the asset is materialized in
// memory: the eBPF loaders parse it as an ELF, which needs io.ReaderAt.
func TestDecompressAssetRandomAccess(t *testing.T) {
	content := testAssetContent()

	reader, err := decompressAsset(".gz", bytes.NewReader(gzipBytes(t, content)), maxDecompressedAssetSize)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, reader.Close()) })

	buf := make([]byte, 16)
	n, err := reader.ReadAt(buf, 1024)
	require.NoError(t, err)
	assert.Equal(t, content[1024:1024+n], buf[:n])
}

func TestDecompressAssetUnsupportedExtension(t *testing.T) {
	_, err := decompressAsset(".zst", bytes.NewReader(nil), maxDecompressedAssetSize)
	assert.ErrorContains(t, err, "unsupported compressed asset extension")
}

func TestDecompressAssetCorrupted(t *testing.T) {
	_, err := decompressAsset(".gz", bytes.NewReader([]byte("not a gzip stream")), maxDecompressedAssetSize)
	assert.Error(t, err)
}

func TestDecompressAssetTooLarge(t *testing.T) {
	content := testAssetContent()

	_, err := decompressAsset(".gz", bytes.NewReader(gzipBytes(t, content)), int64(len(content))-1)
	assert.ErrorContains(t, err, "exceeds")
}
